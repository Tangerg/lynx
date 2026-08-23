// Package runtimeevents owns bounded Runtime resource invalidation streams.
// Resource payloads remain query-owned; this bus carries only committed change
// identities and forces an explicit resync when a subscriber falls behind.
package runtimeevents

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const subscriberCapacity = 128

type subscriber struct {
	mu       sync.Mutex
	topics   []protocol.RuntimeTopic
	watchIDs []string
	sequence uint64
	values   chan protocol.RuntimeEvent
	done     chan struct{}
	cancel   context.CancelFunc
	once     sync.Once
}

func newSubscriber(request protocol.RuntimeSubscribeRequest) *subscriber {
	watchIDs := make([]string, 0, len(request.Watches))
	for _, watch := range request.Watches {
		watchIDs = append(watchIDs, watch.WatchID)
	}
	return &subscriber{
		topics: slices.Clone(request.Topics), watchIDs: watchIDs,
		values: make(chan protocol.RuntimeEvent, subscriberCapacity), done: make(chan struct{}),
	}
}

func (subscriber *subscriber) close() {
	subscriber.once.Do(func() {
		if subscriber.cancel != nil {
			subscriber.cancel()
		}
		close(subscriber.done)
	})
}

func (subscriber *subscriber) accepts(topic protocol.RuntimeTopic) bool {
	return slices.Contains(subscriber.topics, topic)
}

func (subscriber *subscriber) emit(event protocol.RuntimeEvent) {
	subscriber.mu.Lock()
	defer subscriber.mu.Unlock()
	select {
	case <-subscriber.done:
		return
	default:
	}
	subscriber.sequence++
	event.Sequence = subscriber.sequence
	select {
	case subscriber.values <- event:
		return
	default:
	}
	// The queue is no longer a trustworthy sequence. Replace its contents with
	// one authoritative recovery instruction rather than silently dropping a
	// particular resource identity.
	for {
		select {
		case <-subscriber.values:
		default:
			subscriber.sequence++
			subscriber.values <- protocol.RuntimeEvent{
				Type: protocol.RuntimeResync, Sequence: subscriber.sequence,
				Topics: slices.Clone(subscriber.topics), WatchIDs: slices.Clone(subscriber.watchIDs),
			}
			return
		}
	}
}

type Bus struct {
	mu                  sync.Mutex
	nextID              uint64
	subscribers         map[uint64]*subscriber
	userSkillsDirectory string
	closed              bool
	watches             sync.WaitGroup
}

type Config struct {
	// UserSkillsDirectory is optional. When set, subscribers to skills.changed
	// also observe external edits in this absolute, existing directory.
	UserSkillsDirectory string
}

func New(config Config) (*Bus, error) {
	directory := ""
	if config.UserSkillsDirectory != "" {
		if !filepath.IsAbs(config.UserSkillsDirectory) {
			return nil, errors.New("runtimeevents: user Skills directory must be absolute")
		}
		physical, err := filepath.EvalSymlinks(config.UserSkillsDirectory)
		if err != nil {
			return nil, fmt.Errorf("runtimeevents: resolve user Skills directory: %w", err)
		}
		directory = filepath.Clean(physical)
	}
	return &Bus{
		subscribers: make(map[uint64]*subscriber),
		userSkillsDirectory: directory,
	}, nil
}

func (bus *Bus) Publish(event protocol.RuntimeEvent) {
	topic := protocol.RuntimeTopic(event.Type)
	bus.mu.Lock()
	targets := make([]*subscriber, 0, len(bus.subscribers))
	for _, subscriber := range bus.subscribers {
		if subscriber.accepts(topic) {
			targets = append(targets, subscriber)
		}
	}
	bus.mu.Unlock()
	for _, subscriber := range targets {
		subscriber.emit(event)
	}
}

func (bus *Bus) Subscribe(
	ctx context.Context,
	request protocol.RuntimeSubscribeRequest,
) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	if err := validateRequest(request); err != nil {
		return nil, nil, err
	}
	subscription := newSubscriber(request)
	watchContext, cancelWatches := context.WithCancel(ctx)
	subscription.cancel = cancelWatches
	bus.mu.Lock()
	if bus.closed {
		bus.mu.Unlock()
		cancelWatches()
		return nil, nil, errors.New("runtimeevents: bus is closed")
	}
	bus.nextID++
	id := bus.nextID
	bus.subscribers[id] = subscription
	bus.mu.Unlock()

	for _, watch := range request.Watches {
		bus.watches.Add(1)
		go func(spec protocol.WatchSpec) {
			defer bus.watches.Done()
			watchFiles(watchContext, subscription, spec)
		}(watch)
	}
	if subscription.accepts(protocol.TopicSkillsChanged) && bus.userSkillsDirectory != "" {
		bus.watches.Add(1)
		go func() {
			defer bus.watches.Done()
			watchUserSkills(watchContext, subscription, bus.userSkillsDirectory)
		}()
	}
	remove := func() {
		cancelWatches()
		bus.mu.Lock()
		delete(bus.subscribers, id)
		bus.mu.Unlock()
		subscription.close()
	}
	context.AfterFunc(ctx, remove)

	stream := func(yield func(protocol.RuntimeEvent) bool) {
		defer remove()
		for {
			select {
			case event := <-subscription.values:
				if !yield(event) {
					return
				}
			case <-subscription.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}
	return &protocol.RuntimeSubscribeResponse{}, stream, nil
}

func (bus *Bus) Close() {
	bus.mu.Lock()
	if bus.closed {
		bus.mu.Unlock()
		return
	}
	bus.closed = true
	for _, subscriber := range bus.subscribers {
		subscriber.close()
	}
	bus.subscribers = nil
	bus.mu.Unlock()
	bus.watches.Wait()
}

func validateRequest(request protocol.RuntimeSubscribeRequest) error {
	if len(request.Topics) == 0 || len(request.Topics) > protocol.MaxSubscriptionTopics {
		return fmt.Errorf("%w: topics must contain 1..%d entries", protocol.ErrInvalidParams, protocol.MaxSubscriptionTopics)
	}
	known := protocol.RuntimeTopics()
	seenTopics := make(map[protocol.RuntimeTopic]bool, len(request.Topics))
	for _, topic := range request.Topics {
		if !slices.Contains(known, topic) || seenTopics[topic] {
			return fmt.Errorf("%w: invalid or duplicate topic %q", protocol.ErrInvalidParams, topic)
		}
		seenTopics[topic] = true
	}
	if len(request.Watches) > protocol.MaxSubscriptionWatches {
		return fmt.Errorf("%w: too many watches", protocol.ErrInvalidParams)
	}
	if len(request.Watches) > 0 && !seenTopics[protocol.TopicFilesChanged] {
		return fmt.Errorf("%w: watches require files.changed", protocol.ErrInvalidParams)
	}
	seenWatches := make(map[string]bool, len(request.Watches))
	for _, watch := range request.Watches {
		if strings.TrimSpace(watch.WatchID) == "" || strings.TrimSpace(watch.Workspace.Path) == "" || seenWatches[watch.WatchID] {
			return fmt.Errorf("%w: invalid or duplicate watch", protocol.ErrInvalidParams)
		}
		seenWatches[watch.WatchID] = true
	}
	return nil
}

func watchFiles(ctx context.Context, target *subscriber, spec protocol.WatchSpec) {
	root, err := filepath.Abs(spec.Workspace.Path)
	if err != nil {
		target.emit(resyncWatch(spec.WatchID))
		return
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		target.emit(resyncWatch(spec.WatchID))
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		target.emit(resyncWatch(spec.WatchID))
		return
	}
	defer watcher.Close()
	if err := addTree(watcher, root); err != nil {
		target.emit(resyncWatch(spec.WatchID))
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addTree(watcher, event.Name)
				}
			}
			relative, relErr := filepath.Rel(root, event.Name)
			if relErr != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			target.emit(protocol.RuntimeEvent{
				Type: protocol.RuntimeFilesChanged, WatchID: spec.WatchID,
				Workspace: &spec.Workspace, Paths: []string{filepath.ToSlash(relative)},
			})
			if target.accepts(protocol.TopicSkillsChanged) {
				if name, skillChange := workspaceSkillChange(relative); skillChange {
					names := []string(nil)
					if name != "" {
						names = []string{name}
					}
					target.emit(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged, Names: names})
				}
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			target.emit(resyncWatch(spec.WatchID))
		case <-ctx.Done():
			return
		}
	}
}

func watchUserSkills(ctx context.Context, target *subscriber, directory string) {
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	defer watcher.Close()
	if err := addTree(watcher, root); err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	if err := watcher.Add(filepath.Dir(root)); err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			relative, err := filepath.Rel(root, event.Name)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addTree(watcher, event.Name)
				}
			}
			if relative == "." {
				target.emit(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})
				continue
			}
			name := strings.Split(filepath.ToSlash(relative), "/")[0]
			names := []string(nil)
			if name != "" && !strings.HasPrefix(name, ".") {
				names = []string{name}
			}
			target.emit(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged, Names: names})
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			target.emit(resyncTopic(protocol.TopicSkillsChanged))
		case <-ctx.Done():
			return
		}
	}
}

func workspaceSkillChange(relative string) (string, bool) {
	const prefix = ".lyra/skills"
	path := filepath.ToSlash(relative)
	if path == prefix {
		return "", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", false
	}
	remainder := strings.TrimPrefix(path, prefix+"/")
	name := strings.Split(remainder, "/")[0]
	if strings.HasPrefix(name, ".") {
		return "", true
	}
	return name, true
}

func addTree(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if path != root && (name == ".git" || name == "node_modules" || name == ".cache") {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func resyncWatch(id string) protocol.RuntimeEvent {
	return protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		WatchIDs: []string{id},
	}
}

func resyncTopic(topic protocol.RuntimeTopic) protocol.RuntimeEvent {
	return protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{topic}}
}
