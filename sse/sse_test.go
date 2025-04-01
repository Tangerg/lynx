package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

func newServer() {
	http.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		eventChan := make(chan *Message)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WithSSE(ctx, w, eventChan)
			fmt.Println("sse stop")
			fmt.Println(err)
		}()
		time.Sleep(1 * time.Second)
		for i := 0; i < 100; i++ {
			itoa := strconv.Itoa(i + 1)
			data := map[string]any{
				"id":         itoa,
				"time_stamp": time.Now().Unix(),
			}
			marshal, _ := json.Marshal(data)
			eventChan <- &Message{
				ID:    itoa,
				Data:  marshal,
				Event: "event_" + itoa,
				Retry: 0,
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(eventChan)
		wg.Wait()
	})
	_ = http.ListenAndServe(":8080", nil)
}

func newServer2() {
	http.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		writer, err := NewWriter(&WriterConfig{
			Context:        ctx,
			ResponseWriter: w,
			QueueSize:      128,
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		defer writer.Close()
		time.Sleep(1 * time.Second)
		for i := 0; i < 100; i++ {
			itoa := strconv.Itoa(i + 1)
			data := map[string]any{
				"id":         itoa,
				"time_stamp": time.Now().Unix(),
			}
			marshal, _ := json.Marshal(data)
			writer.Send(&Message{
				ID:    itoa,
				Data:  marshal,
				Event: "event_" + itoa,
				Retry: 0,
			})
			time.Sleep(100 * time.Millisecond)
		}
	})
	_ = http.ListenAndServe(":8080", nil)
}

func newServer3() {
	http.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		writer, err := NewWriter(&WriterConfig{
			Context:        ctx,
			ResponseWriter: w,
			QueueSize:      128,
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		defer writer.Close()
		time.Sleep(1 * time.Second)
		for i := 0; i < 100; i++ {
			itoa := strconv.Itoa(i + 1)
			writer.Send(&Message{
				ID:    itoa,
				Data:  []byte("test\n\n\ntest"),
				Event: "event_" + itoa,
				Retry: 0,
			})
			time.Sleep(500 * time.Millisecond)
		}
	})
	_ = http.ListenAndServe(":8080", nil)
}

func TestSSE2(t *testing.T) {
	go func() {
		newServer()
	}()

	time.Sleep(2 * time.Second)
	resp, err := http.Get("http://localhost:8080/sse")
	if err != nil {
		t.Fatal(err)
	}

	reader := NewReader(resp)
	t.Log(reader.LastID())
	for reader.Next() {
		t.Log(reader.LastID())
		current, err := reader.Current()
		if err != nil {
			t.Log(err)
		}
		var str map[string]any
		err = json.Unmarshal(current.Data, &str)
		if err != nil {
			t.Log(err)
		}
		t.Log(current.ID, current.Event, str)
	}
	reader.Close()
	time.Sleep(1 * time.Second)
}

func Test3(t *testing.T) {
	go func() {
		newServer2()
	}()

	time.Sleep(2 * time.Second)
	resp, err := http.Get("http://localhost:8080/sse")
	if err != nil {
		t.Fatal(err)
	}

	reader := NewReader(resp)
	t.Log(reader.LastID())
	for reader.Next() {
		t.Log(reader.LastID())
		current, err := reader.Current()
		if err != nil {
			t.Log(err)
		}
		var str map[string]any
		err = json.Unmarshal(current.Data, &str)
		if err != nil {
			t.Log(err)
		}
		t.Log(current.ID, current.Event, str)
	}
	reader.Close()
	time.Sleep(1 * time.Second)
}

func Test4(t *testing.T) {
	go func() {
		newServer3()
	}()

	time.Sleep(2 * time.Second)
	resp, err := http.Get("http://localhost:8080/sse")
	if err != nil {
		t.Fatal(err)
	}

	reader := NewReader(resp)
	t.Log(reader.LastID())
	for reader.Next() {
		t.Log(reader.LastID())
		current, err := reader.Current()
		if err != nil {
			t.Log(err)
		}

		t.Log(current.ID, current.Event, string(current.Data))
	}
	reader.Close()
	time.Sleep(1 * time.Second)
}
