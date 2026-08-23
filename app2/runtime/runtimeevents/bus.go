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

const (
	subscriberCapacity         = 128
	maxDirectoriesPerFileWatch = 8_192
)

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

type watchStartup struct {
	once  sync.Once
	ready chan struct{}
}

func newWatchStartup() *watchStartup {
	return &watchStartup{ready: make(chan struct{})}
}

func (startup *watchStartup) markReady() {
	startup.once.Do(func() { close(startup.ready) })
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
	knowledgeFiles      KnowledgeFileSource
	hookFiles           HookFileSource
	closed              bool
	watches             sync.WaitGroup
}

type KnowledgeFileSource interface {
	KnowledgeFiles(context.Context, []protocol.WorkspaceRef) ([]string, error)
}

type HookFileSource interface {
	HookFiles(context.Context, []protocol.WorkspaceRef) ([]string, error)
}

type Config struct {
	// UserSkillsDirectory is optional. When set, subscribers to skills.changed
	// also observe external edits in this absolute, existing directory.
	UserSkillsDirectory string
	// KnowledgeFiles resolves the home and selected-workspace LYRA.md documents
	// observed for subscribers to knowledge.changed.
	KnowledgeFiles KnowledgeFileSource
	// HookFiles resolves global and selected-workspace hooks.json documents.
	HookFiles HookFileSource
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
		subscribers:         make(map[uint64]*subscriber),
		userSkillsDirectory: directory,
		knowledgeFiles:      config.KnowledgeFiles,
		hookFiles:           config.HookFiles,
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
	remove := func() {
		cancelWatches()
		bus.mu.Lock()
		delete(bus.subscribers, id)
		bus.mu.Unlock()
		subscription.close()
	}
	context.AfterFunc(ctx, remove)
	startups := make([]*watchStartup, 0, len(request.Watches)+3)

	for _, watch := range request.Watches {
		if bus.beginWatch() {
			startup := newWatchStartup()
			startups = append(startups, startup)
			go func(spec protocol.WatchSpec) {
				defer bus.watches.Done()
				watchFiles(watchContext, subscription, spec, startup)
			}(watch)
		}
	}
	if subscription.accepts(protocol.TopicSkillsChanged) && bus.userSkillsDirectory != "" {
		if bus.beginWatch() {
			startup := newWatchStartup()
			startups = append(startups, startup)
			go func() {
				defer bus.watches.Done()
				watchUserSkills(watchContext, subscription, bus.userSkillsDirectory, startup)
			}()
		}
	}
	if subscription.accepts(protocol.TopicKnowledgeChanged) && bus.knowledgeFiles != nil {
		workspaces := make([]protocol.WorkspaceRef, 0, len(request.Watches))
		for _, watch := range request.Watches {
			workspaces = append(workspaces, watch.Workspace)
		}
		files, filesErr := bus.knowledgeFiles.KnowledgeFiles(watchContext, workspaces)
		if filesErr != nil {
			subscription.emit(resyncTopic(protocol.TopicKnowledgeChanged))
		} else if len(files) > 0 && bus.beginWatch() {
			startup := newWatchStartup()
			startups = append(startups, startup)
			go func() {
				defer bus.watches.Done()
				watchExactFiles(
					watchContext,
					subscription,
					files,
					protocol.TopicKnowledgeChanged,
					startup,
				)
			}()
		}
	}
	if subscription.accepts(protocol.TopicHooksChanged) && bus.hookFiles != nil {
		workspaces := make([]protocol.WorkspaceRef, 0, len(request.Watches))
		for _, watch := range request.Watches {
			workspaces = append(workspaces, watch.Workspace)
		}
		files, filesErr := bus.hookFiles.HookFiles(watchContext, workspaces)
		if filesErr != nil {
			subscription.emit(resyncTopic(protocol.TopicHooksChanged))
		} else if len(files) > 0 && bus.beginWatch() {
			startup := newWatchStartup()
			startups = append(startups, startup)
			go func() {
				defer bus.watches.Done()
				watchExactFiles(
					watchContext,
					subscription,
					files,
					protocol.TopicHooksChanged,
					startup,
				)
			}()
		}
	}
	for _, startup := range startups {
		select {
		case <-startup.ready:
		case <-ctx.Done():
			remove()
			return nil, nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		remove()
		return nil, nil, err
	}
	// Register first, make every external watcher ready, then force one cold
	// read. This closes the query-before-subscribe and reconnect windows without
	// turning the event stream into a second resource snapshot.
	subscription.emit(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: slices.Clone(request.Topics),
		WatchIDs: slices.Clone(subscription.watchIDs),
	})

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

// beginWatch makes WaitGroup registration atomic with the closed check. Close
// may therefore wait without racing a late Subscribe Add.
func (bus *Bus) beginWatch() bool {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if bus.closed {
		return false
	}
	bus.watches.Add(1)
	return true
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

func watchFiles(
	ctx context.Context,
	target *subscriber,
	spec protocol.WatchSpec,
	startup *watchStartup,
) {
	defer startup.markReady()
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
	observed := make(map[string]struct{})
	if err := addTree(ctx, watcher, root, observed); err != nil {
		target.emit(resyncWatch(spec.WatchID))
		return
	}
	startup.markReady()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					if err := addTree(ctx, watcher, event.Name, observed); err != nil {
						target.emit(resyncWatch(spec.WatchID))
						return
					}
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

func watchUserSkills(
	ctx context.Context,
	target *subscriber,
	directory string,
	startup *watchStartup,
) {
	defer startup.markReady()
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
	observed := make(map[string]struct{})
	if err := addTree(ctx, watcher, root, observed); err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	if err := addWatchDirectory(watcher, filepath.Dir(root), observed); err != nil {
		target.emit(resyncTopic(protocol.TopicSkillsChanged))
		return
	}
	startup.markReady()
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
					if err := addTree(ctx, watcher, event.Name, observed); err != nil {
						target.emit(resyncTopic(protocol.TopicSkillsChanged))
						return
					}
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

func watchExactFiles(
	ctx context.Context,
	target *subscriber,
	files []string,
	topic protocol.RuntimeTopic,
	startup *watchStartup,
) {
	defer startup.markReady()
	targets := make(map[string]struct{}, len(files))
	directories := make(map[string]struct{}, len(files))
	for _, path := range files {
		if !filepath.IsAbs(path) {
			target.emit(resyncTopic(topic))
			return
		}
		path = filepath.Clean(path)
		targets[path] = struct{}{}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		target.emit(resyncTopic(topic))
		return
	}
	defer watcher.Close()
	if err := refreshExactWatchDirectories(watcher, targets, directories); err != nil {
		target.emit(resyncTopic(topic))
		return
	}
	startup.markReady()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			path := filepath.Clean(event.Name)
			if exactTargetAffected(path, targets) {
				target.emit(protocol.RuntimeEvent{Type: protocol.RuntimeEventType(topic)})
			}
			if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					delete(directories, path)
				}
				if err := refreshExactWatchDirectories(
					watcher,
					targets,
					directories,
				); err != nil {
					target.emit(resyncTopic(topic))
					return
				}
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			target.emit(resyncTopic(topic))
		case <-ctx.Done():
			return
		}
	}
}

func exactTargetAffected(path string, targets map[string]struct{}) bool {
	if _, exact := targets[path]; exact {
		return true
	}
	for target := range targets {
		relative, err := filepath.Rel(path, target)
		if err == nil && relative != "." && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func refreshExactWatchDirectories(
	watcher *fsnotify.Watcher,
	targets map[string]struct{},
	observed map[string]struct{},
) error {
	for target := range targets {
		directory := filepath.Dir(target)
		for {
			info, err := os.Stat(directory)
			if err == nil && info.IsDir() {
				break
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				return fmt.Errorf("runtimeevents: exact file %s has no observable ancestor", target)
			}
			directory = parent
		}
		physical, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return err
		}
		physical = filepath.Clean(physical)
		if physical != directory && !exactTargetBelow(physical, targets) {
			return fmt.Errorf(
				"runtimeevents: exact file %s changed physical identity",
				target,
			)
		}
		directory = physical
		if _, exists := observed[directory]; exists {
			continue
		}
		if err := addWatchDirectory(watcher, directory, observed); err != nil {
			return err
		}
	}
	return nil
}

func exactTargetBelow(directory string, targets map[string]struct{}) bool {
	for target := range targets {
		relative, err := filepath.Rel(directory, target)
		if err == nil && relative != "." && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
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

func addTree(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	root string,
	observed map[string]struct{},
) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		if _, exists := observed[path]; exists {
			return filepath.SkipDir
		}
		return addWatchDirectory(watcher, path, observed)
	})
}

func addWatchDirectory(
	watcher *fsnotify.Watcher,
	path string,
	observed map[string]struct{},
) error {
	path = filepath.Clean(path)
	if _, exists := observed[path]; exists {
		return nil
	}
	if len(observed) >= maxDirectoriesPerFileWatch {
		return fmt.Errorf(
			"runtimeevents: file watch exceeds %d directories",
			maxDirectoriesPerFileWatch,
		)
	}
	if err := watcher.Add(path); err != nil {
		return err
	}
	observed[path] = struct{}{}
	return nil
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
