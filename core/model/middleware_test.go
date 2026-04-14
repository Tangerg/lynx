package model

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
)

// ==================== MiddlewareManager Tests ====================

func TestMiddlewareManager_CallMiddleware(t *testing.T) {
	t.Run("single middleware", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req+"_modified")
				if err != nil {
					return "", err
				}
				return resp + "_wrapped", nil
			})
		}

		manager.UseCallMiddlewares(middleware)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "response_" + req, nil
		})

		handler := manager.BuildCallHandler(endpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "response_test_modified_wrapped"
		if resp != expected {
			t.Errorf("expected %q, got %q", expected, resp)
		}
	})

	t.Run("multiple middlewares execution order", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		var executionOrder []string
		mu := sync.Mutex{}

		middleware1 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				mu.Lock()
				executionOrder = append(executionOrder, "m1_before")
				mu.Unlock()

				resp, err := next.Call(ctx, req)

				mu.Lock()
				executionOrder = append(executionOrder, "m1_after")
				mu.Unlock()

				return resp, err
			})
		}

		middleware2 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				mu.Lock()
				executionOrder = append(executionOrder, "m2_before")
				mu.Unlock()

				resp, err := next.Call(ctx, req)

				mu.Lock()
				executionOrder = append(executionOrder, "m2_after")
				mu.Unlock()

				return resp, err
			})
		}

		manager.UseCallMiddlewares(middleware1, middleware2)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "endpoint")
			mu.Unlock()
			return "response", nil
		})

		handler := manager.BuildCallHandler(endpoint)
		_, _ = handler.Call(context.Background(), "test")

		// 验证执行顺序：m1 -> m2 -> endpoint -> m2 -> m1
		expected := []string{"m1_before", "m2_before", "endpoint", "m2_after", "m1_after"}
		if len(executionOrder) != len(expected) {
			t.Fatalf("expected %d execution steps, got %d", len(expected), len(executionOrder))
		}
		for i, step := range expected {
			if executionOrder[i] != step {
				t.Errorf("step %d: expected %q, got %q", i, step, executionOrder[i])
			}
		}
	})

	t.Run("middleware modifies request and response", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		middleware1 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m1(" + resp + ")", nil
			})
		}

		middleware2 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m2(" + resp + ")", nil
			})
		}

		manager.UseCallMiddlewares(middleware1, middleware2)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "endpoint(" + req + ")", nil
		})

		handler := manager.BuildCallHandler(endpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// m1包装m2的结果，m2包装endpoint的结果
		expected := "m1(m2(endpoint(test)))"
		if resp != expected {
			t.Errorf("expected %q, got %q", expected, resp)
		}
	})

	t.Run("nil middleware ignored", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		manager.UseCallMiddlewares(nil, nil)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "response", nil
		})

		handler := manager.BuildCallHandler(endpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != "response" {
			t.Errorf("expected 'response', got %q", resp)
		}
	})

	t.Run("empty middleware list", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		manager.UseCallMiddlewares()

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "response", nil
		})

		handler := manager.BuildCallHandler(endpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != "response" {
			t.Errorf("expected 'response', got %q", resp)
		}
	})

	t.Run("middleware error handling", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		expectedErr := errors.New("middleware error")
		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				return "", expectedErr
			})
		}

		manager.UseCallMiddlewares(middleware)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "should not reach", nil
		})

		handler := manager.BuildCallHandler(endpoint)
		_, err := handler.Call(context.Background(), "test")
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("method chaining", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		result := manager.
			UseCallMiddlewares(func(h CallHandler[string, string]) CallHandler[string, string] { return h }).
			UseCallMiddlewares(func(h CallHandler[string, string]) CallHandler[string, string] { return h })

		if result != manager {
			t.Error("UseCallMiddlewares should return the same manager instance")
		}
	})
}

func TestMiddlewareManager_StreamMiddleware(t *testing.T) {
	t.Run("single stream middleware", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		middleware := func(next StreamHandler[string, string]) StreamHandler[string, string] {
			return StreamHandlerFunc[string, string](func(ctx context.Context, req string) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					for chunk, err := range next.Stream(ctx, req+"_modified") {
						if err != nil {
							yield("", err)
							return
						}
						if !yield(chunk+"_wrapped", nil) {
							return
						}
					}
				}
			})
		}

		manager.UseStreamMiddlewares(middleware)

		endpoint := StreamHandlerFunc[string, string](func(ctx context.Context, req string) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				chunks := []string{"a", "b", "c"}
				for _, chunk := range chunks {
					if !yield(req+"_"+chunk, nil) {
						return
					}
				}
			}
		})

		handler := manager.BuildStreamHandler(endpoint)

		var results []string
		for chunk, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			results = append(results, chunk)
		}

		expected := []string{"test_modified_a_wrapped", "test_modified_b_wrapped", "test_modified_c_wrapped"}
		if len(results) != len(expected) {
			t.Fatalf("expected %d chunks, got %d", len(expected), len(results))
		}
		for i, chunk := range results {
			if chunk != expected[i] {
				t.Errorf("chunk %d: expected %q, got %q", i, expected[i], chunk)
			}
		}
	})

	t.Run("multiple stream middlewares", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, int]()

		callCount := atomic.Int32{}

		middleware1 := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						callCount.Add(1)
						if !yield(chunk*2, nil) {
							return
						}
					}
				}
			})
		}

		middleware2 := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk+10, nil) {
							return
						}
					}
				}
			})
		}

		manager.UseStreamMiddlewares(middleware1, middleware2)

		endpoint := StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for i := 1; i <= 3; i++ {
					if !yield(i, nil) {
						return
					}
				}
			}
		})

		handler := manager.BuildStreamHandler(endpoint)

		var results []int
		for chunk, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			results = append(results, chunk)
		}

		// middleware1先执行，对middleware2的结果*2
		// middleware2对endpoint的结果+10
		// 所以: (1+10)*2=22, (2+10)*2=24, (3+10)*2=26
		expected := []int{22, 24, 26}
		if len(results) != len(expected) {
			t.Fatalf("expected %d chunks, got %d", len(expected), len(results))
		}
		for i, chunk := range results {
			if chunk != expected[i] {
				t.Errorf("chunk %d: expected %d, got %d", i, expected[i], chunk)
			}
		}

		if callCount.Load() != 3 {
			t.Errorf("expected middleware1 to be called 3 times, got %d", callCount.Load())
		}
	})

	t.Run("stream error propagation", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		expectedErr := errors.New("stream error")

		middleware := func(next StreamHandler[string, string]) StreamHandler[string, string] {
			return StreamHandlerFunc[string, string](func(ctx context.Context, req string) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield("", fmt.Errorf("wrapped: %w", err))
							return
						}
						if !yield(chunk, nil) {
							return
						}
					}
				}
			})
		}

		manager.UseStreamMiddlewares(middleware)

		endpoint := StreamHandlerFunc[string, string](func(ctx context.Context, req string) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("chunk1", nil)
				yield("", expectedErr)
			}
		})

		handler := manager.BuildStreamHandler(endpoint)

		chunks := 0
		var gotErr error
		for _, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				gotErr = err
				break
			}
			chunks++
		}

		if chunks != 1 {
			t.Errorf("expected 1 chunk before error, got %d", chunks)
		}
		if gotErr == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(gotErr, expectedErr) {
			t.Errorf("expected error to wrap %v, got %v", expectedErr, gotErr)
		}
	})

	t.Run("nil stream middleware ignored", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		manager.UseStreamMiddlewares(nil, nil)

		endpoint := StreamHandlerFunc[string, string](func(ctx context.Context, req string) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				yield("chunk", nil)
			}
		})

		handler := manager.BuildStreamHandler(endpoint)

		var results []string
		for chunk, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			results = append(results, chunk)
		}

		if len(results) != 1 || results[0] != "chunk" {
			t.Errorf("expected ['chunk'], got %v", results)
		}
	})
}

func TestMiddlewareManager_UseMiddlewares(t *testing.T) {
	t.Run("mixed middleware types", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, int]()

		callMiddleware := CallMiddleware[string, string](func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "call_" + resp, nil
			})
		})

		streamMiddleware := StreamMiddleware[string, int](func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk*10, nil) {
							return
						}
					}
				}
			})
		})

		manager.UseMiddlewares(callMiddleware, streamMiddleware)

		// Test call middleware
		callEndpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "original", nil
		})
		callHandler := manager.BuildCallHandler(callEndpoint)
		resp, _ := callHandler.Call(context.Background(), "test")
		if resp != "call_original" {
			t.Errorf("call middleware not applied correctly, got %q", resp)
		}

		// Test stream middleware
		streamEndpoint := StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				yield(5, nil)
			}
		})
		streamHandler := manager.BuildStreamHandler(streamEndpoint)
		for chunk, err := range streamHandler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if chunk != 50 {
				t.Errorf("stream middleware not applied correctly, got %d", chunk)
			}
			break
		}
	})

	t.Run("nil and invalid types ignored", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		manager.UseMiddlewares(nil, "invalid", 123, nil)

		callEndpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "response", nil
		})

		handler := manager.BuildCallHandler(callEndpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != "response" {
			t.Errorf("expected 'response', got %q", resp)
		}
	})

	t.Run("empty middleware list", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		result := manager.UseMiddlewares()

		if result != manager {
			t.Error("UseMiddlewares should return the same manager instance")
		}
	})
}

func TestMiddlewareManager_Clone(t *testing.T) {
	t.Run("clone creates independent copy", func(t *testing.T) {
		original := NewMiddlewareManager[string, string, string, string]()

		middleware1 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				res, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m1_" + res, nil
			})
		}

		original.UseCallMiddlewares(middleware1)

		cloned := original.Clone()

		middleware2 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				res, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m2_" + res, nil
			})
		}

		cloned.UseCallMiddlewares(middleware2)

		// Original should only have middleware1
		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return req, nil
		})

		originalHandler := original.BuildCallHandler(endpoint)
		resp1, _ := originalHandler.Call(context.Background(), "test")
		if resp1 != "m1_test" {
			t.Errorf("original manager: expected 'm1_test', got %q", resp1)
		}

		// Cloned should have both middleware1 and middleware2
		clonedHandler := cloned.BuildCallHandler(endpoint)
		resp2, _ := clonedHandler.Call(context.Background(), "test")
		if resp2 != "m1_m2_test" {
			t.Errorf("cloned manager: expected 'm1_m2_test', got %q", resp2)
		}
	})

	t.Run("clone nil manager", func(t *testing.T) {
		var manager *MiddlewareManager[string, string, string, string]
		cloned := manager.Clone()
		if cloned != nil {
			t.Error("cloning nil manager should return nil")
		}
	})

	t.Run("clone preserves stream middlewares", func(t *testing.T) {
		original := NewMiddlewareManager[string, string, string, int]()

		middleware := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk*2, nil) {
							return
						}
					}
				}
			})
		}

		original.UseStreamMiddlewares(middleware)

		cloned := original.Clone()

		endpoint := StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				yield(21, nil)
			}
		})

		handler := cloned.BuildStreamHandler(endpoint)
		for chunk, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if chunk != 42 {
				t.Errorf("expected 42, got %d", chunk)
			}
			break
		}
	})
}

func TestMiddlewareManager_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent UseMiddlewares", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		var wg sync.WaitGroup
		const goroutines = 10

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
					return next
				}
				manager.UseCallMiddlewares(middleware)
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent Build and Use", func(t *testing.T) {
		manager := NewMiddlewareManager[string, string, string, string]()

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "response", nil
		})

		var wg sync.WaitGroup
		const goroutines = 20

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				if id%2 == 0 {
					middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
						return next
					}
					manager.UseCallMiddlewares(middleware)
				} else {
					handler := manager.BuildCallHandler(endpoint)
					_, _ = handler.Call(context.Background(), "test")
				}
			}(i)
		}

		wg.Wait()
	})
}

// ==================== CallMiddlewareManager Tests ====================

func TestCallMiddlewareManager(t *testing.T) {
	t.Run("basic functionality", func(t *testing.T) {
		manager := NewCallMiddlewareManager[string, string]()

		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "wrapped_" + resp, nil
			})
		}

		manager.UseMiddlewares(middleware)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return req, nil
		})

		handler := manager.BuildHandler(endpoint)
		resp, err := handler.Call(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != "wrapped_test" {
			t.Errorf("expected 'wrapped_test', got %q", resp)
		}
	})

	t.Run("method chaining", func(t *testing.T) {
		manager := NewCallMiddlewareManager[string, string]()

		result := manager.
			UseMiddlewares(func(h CallHandler[string, string]) CallHandler[string, string] { return h }).
			UseMiddlewares(func(h CallHandler[string, string]) CallHandler[string, string] { return h })

		if result != manager {
			t.Error("UseMiddlewares should return the same manager instance")
		}
	})

	t.Run("clone", func(t *testing.T) {
		original := NewCallMiddlewareManager[string, string]()

		middleware1 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m1(" + resp + ")", nil
			})
		}

		original.UseMiddlewares(middleware1)

		cloned := original.Clone()

		middleware2 := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				resp, err := next.Call(ctx, req)
				if err != nil {
					return "", err
				}
				return "m2(" + resp + ")", nil
			})
		}

		cloned.UseMiddlewares(middleware2)

		endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return req, nil
		})

		// Original: m1(test)
		originalHandler := original.BuildHandler(endpoint)
		resp1, _ := originalHandler.Call(context.Background(), "test")
		if resp1 != "m1(test)" {
			t.Errorf("original: expected 'm1(test)', got %q", resp1)
		}

		// Cloned: m1(m2(test))
		clonedHandler := cloned.BuildHandler(endpoint)
		resp2, _ := clonedHandler.Call(context.Background(), "test")
		if resp2 != "m1(m2(test))" {
			t.Errorf("cloned: expected 'm1(m2(test))', got %q", resp2)
		}
	})

	t.Run("clone nil manager", func(t *testing.T) {
		var manager *CallMiddlewareManager[string, string]
		cloned := manager.Clone()
		if cloned != nil {
			t.Error("cloning nil manager should return nil")
		}
	})
}

// ==================== StreamMiddlewareManager Tests ====================

func TestStreamMiddlewareManager(t *testing.T) {
	t.Run("basic functionality", func(t *testing.T) {
		manager := NewStreamMiddlewareManager[string, int]()

		middleware := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk*10, nil) {
							return
						}
					}
				}
			})
		}

		manager.UseMiddlewares(middleware)

		endpoint := StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for i := 1; i <= 3; i++ {
					if !yield(i, nil) {
						return
					}
				}
			}
		})

		handler := manager.BuildHandler(endpoint)

		var results []int
		for chunk, err := range handler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			results = append(results, chunk)
		}

		expected := []int{10, 20, 30}
		if len(results) != len(expected) {
			t.Fatalf("expected %d chunks, got %d", len(expected), len(results))
		}
		for i, chunk := range results {
			if chunk != expected[i] {
				t.Errorf("chunk %d: expected %d, got %d", i, expected[i], chunk)
			}
		}
	})

	t.Run("method chaining", func(t *testing.T) {
		manager := NewStreamMiddlewareManager[string, string]()

		result := manager.
			UseMiddlewares(func(h StreamHandler[string, string]) StreamHandler[string, string] { return h }).
			UseMiddlewares(func(h StreamHandler[string, string]) StreamHandler[string, string] { return h })

		if result != manager {
			t.Error("UseMiddlewares should return the same manager instance")
		}
	})

	t.Run("clone", func(t *testing.T) {
		original := NewStreamMiddlewareManager[string, int]()

		middleware1 := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk*2, nil) {
							return
						}
					}
				}
			})
		}

		original.UseMiddlewares(middleware1)

		cloned := original.Clone()

		middleware2 := func(next StreamHandler[string, int]) StreamHandler[string, int] {
			return StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
				return func(yield func(int, error) bool) {
					for chunk, err := range next.Stream(ctx, req) {
						if err != nil {
							yield(0, err)
							return
						}
						if !yield(chunk+10, nil) {
							return
						}
					}
				}
			})
		}

		cloned.UseMiddlewares(middleware2)

		endpoint := StreamHandlerFunc[string, int](func(ctx context.Context, req string) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				yield(5, nil)
			}
		})

		// Original: 5*2=10
		originalHandler := original.BuildHandler(endpoint)
		for chunk, err := range originalHandler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if chunk != 10 {
				t.Errorf("original: expected 10, got %d", chunk)
			}
			break
		}

		// Cloned: (5+10)*2=30
		clonedHandler := cloned.BuildHandler(endpoint)
		for chunk, err := range clonedHandler.Stream(context.Background(), "test") {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if chunk != 30 {
				t.Errorf("cloned: expected 30, got %d", chunk)
			}
			break
		}
	})

	t.Run("clone nil manager", func(t *testing.T) {
		var manager *StreamMiddlewareManager[string, int]
		cloned := manager.Clone()
		if cloned != nil {
			t.Error("cloning nil manager should return nil")
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkMiddlewareManager_BuildCallHandler(b *testing.B) {
	manager := NewMiddlewareManager[string, string, string, string]()

	for i := 0; i < 5; i++ {
		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				return next.Call(ctx, req)
			})
		}
		manager.UseCallMiddlewares(middleware)
	}

	endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
		return "response", nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.BuildCallHandler(endpoint)
	}
}

func BenchmarkMiddlewareManager_CallExecution(b *testing.B) {
	manager := NewMiddlewareManager[string, string, string, string]()

	for i := 0; i < 5; i++ {
		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
				return next.Call(ctx, req)
			})
		}
		manager.UseCallMiddlewares(middleware)
	}

	endpoint := CallHandlerFunc[string, string](func(ctx context.Context, req string) (string, error) {
		return "response", nil
	})

	handler := manager.BuildCallHandler(endpoint)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = handler.Call(ctx, "test")
	}
}

func BenchmarkMiddlewareManager_Clone(b *testing.B) {
	manager := NewMiddlewareManager[string, string, string, string]()

	for i := 0; i < 10; i++ {
		middleware := func(next CallHandler[string, string]) CallHandler[string, string] {
			return next
		}
		manager.UseCallMiddlewares(middleware)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.Clone()
	}
}
