package datastore

import (
	"context"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCacheGetOrSet(t *testing.T) {
	c := newCache()
	got, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error: %v", err)
	}
	expect := []string{"value"}
	if len(got) != 1 || string(got["key1"]) != expect[0] {
		t.Errorf("GetOrSet() returned `%v`, expected `%v`", got, expect)
	}
}

func TestCacheGetOrSetMissingKeys(t *testing.T) {
	c := newCache()
	got, err := c.GetOrSet(context.Background(), []string{"key1", "key2"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error: %v", err)
	}
	expect := map[string][]byte{"key1": []byte("value"), "key2": nil}
	require.Equal(t, expect, got)
}

func TestCacheGetOrSetNoSecondCall(t *testing.T) {
	c := newCache()
	c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value"))
		return nil
	})

	var called bool

	got, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		called = true
		set("key1", []byte("value"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error %v", err)
	}

	if len(got) != 1 || string(got["key1"]) != "value" {
		t.Errorf("GetOrSet() returned %q, expected %q", got, "value")
	}
	if called {
		t.Errorf("GetOrSet() called the set method")
	}
}

func TestCacheGetOrSetBlockSecondCall(t *testing.T) {
	c := newCache()
	wait := make(chan struct{})
	go func() {
		c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
			<-wait
			set("key1", []byte("value"))
			return nil
		})
	}()

	// close done, when the second call is finished.
	done := make(chan struct{})
	go func() {
		c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
			set("key1", []byte("Shut not be returned"))
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
		t.Errorf("done channel already closed")
	default:
	}

	close(wait)

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Errorf("Second GetOrSet-Call was not done one Millisecond after the frist GetOrSet-Call was called.")
	}
}

func TestCacheSetIfExist(t *testing.T) {
	c := newCache()
	c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("Shut not be returned"))
		return nil
	})

	// Set key1 and key2. key1 is in the cache. key2 should be ignored.
	c.SetIfExistMany(map[string][]byte{
		"key1": []byte("new_value"),
		"key2": []byte("new_value"),
	})

	// Get key1 and key2 from the cache. The existing key1 should not be set.
	// key2 should be.
	got, _ := c.GetOrSet(context.Background(), []string{"key1", "key2"}, func(keys []string, set func(string, []byte)) error {
		for _, key := range keys {
			set(key, []byte(key))
		}
		return nil
	})

	expect := []string{"new_value", "key2"}
	if len(got) != 2 || string(got["key1"]) != expect[0] || string(got["key2"]) != expect[1] {
		t.Errorf("Got %v, expected %v", got, expect)
	}
}

func TestCacheSetIfExistParallelToGetOrSet(t *testing.T) {
	c := newCache()

	waitForGetOrSet := make(chan struct{})
	go func() {
		c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
			// Signal, that GetOrSet was called.
			close(waitForGetOrSet)

			// Wait for some time.
			time.Sleep(10 * time.Millisecond)
			set("key1", []byte("shut not be used"))
			return nil
		})
	}()

	<-waitForGetOrSet

	// Set key1 to new value and stop the ongoing GetOrSet-Call
	c.SetIfExistMany(map[string][]byte{"key1": []byte("new value")})

	got, _ := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("Expect values in cache"))
		return nil
	})

	expect := []string{"new value"}
	if len(got) != 1 || string(got["key1"]) != expect[0] {
		t.Errorf("Got `%s`, expected `%s`", got, expect)
	}
}

func TestCacheGetOrSetOldData(t *testing.T) {
	// GetOrSet is called with key1. It returns key1 and key2 on version1 but
	// takes a long time. In the meantime there is an update via setIfExist for
	// key1 and key2 on version2. At the end, there should not be the old
	// version1 in the cache (version2 or 'does not exist' is ok).
	c := newCache()

	waitForGetOrSetStart := make(chan struct{})
	waitForGetOrSetEnd := make(chan struct{})
	waitForSetIfExist := make(chan struct{})

	go func() {
		c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
			close(waitForGetOrSetStart)
			set("key1", []byte("v1"))
			set("key2", []byte("v1"))
			<-waitForSetIfExist
			return nil
		})
		close(waitForGetOrSetEnd)
	}()

	<-waitForGetOrSetStart
	c.SetIfExistMany(map[string][]byte{
		"key1": []byte("v2"),
		"key2": []byte("v2"),
	})
	close(waitForSetIfExist)

	<-waitForGetOrSetEnd
	data, err := c.GetOrSet(context.Background(), []string{"key1", "key2"}, func(keys []string, set func(string, []byte)) error {
		for _, key := range keys {
			set(key, []byte("key not in cache"))
		}
		return nil
	})
	if err != nil {
		t.Errorf("GetOrSet returned unexpected error: %v", err)
	}

	if string(data["key1"]) != "v2" {
		t.Errorf("value for key1 is %s, expected `v2`", data["key1"])
	}

	if string(data["keys2"]) == "v1" {
		t.Errorf("value for key2 is `v1`, expected `v2` or `key not in cache`")
	}
}

func TestCacheErrorOnFetching(t *testing.T) {
	// Make sure, that if a GetOrSet call fails the requested keys are not left
	// in pending state.
	c := newCache()
	rErr := errors.New("GetOrSet Error")
	_, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		return rErr
	})

	if !errors.Is(err, rErr) {
		t.Errorf("GetOrSet returned err `%v`, expected `%v`", err, rErr)
	}

	done := make(chan map[string][]byte)
	go func() {
		data, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
			set("key1", []byte("value"))
			return nil
		})
		if err != nil {
			t.Errorf("Second GetOrSet returned unexpected err: %v", err)
		}
		done <- data
	}()

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case data := <-done:
		if string(data["key1"]) != "value" {
			t.Errorf("Second GetOrSet-Call returned value %q, expected value", data["key1"])
		}
	case <-timer.C:
		t.Errorf("Second GetOrSet-Call was not done after one Millisecond")
	}
}

func TestCacheConcurency(t *testing.T) {
	const count = 100
	c := newCache()
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			v, err := c.GetOrSet(context.Background(), []string{"key1"}, func(keys []string, set func(k string, v []byte)) error {
				log.Printf("called from %d", i)
				time.Sleep(time.Millisecond)
				for _, k := range keys {
					set(k, []byte("value"))
				}
				return nil
			})
			if err != nil {
				t.Errorf("goroutine %d returned error: %v", i, err)
			}

			if string(v["key1"]) != "value" {
				t.Errorf("goroutine %d returned %q", i, v)
			}

		}(i)
	}

	wg.Wait()
}

func TestGetNull(t *testing.T) {
	c := newCache()
	got, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("null"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error: %v", err)
	}

	if k1, ok := got["key1"]; k1 != nil || !ok {
		t.Errorf("GetOrSet() returned (%q, %t) for key1, expected (nil, true)", k1, ok)
	}
}

func TestUpdateNull(t *testing.T) {
	c := newCache()
	c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value"))
		return nil
	})

	c.SetIfExist("key1", []byte("null"))

	got, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value that should not be fetched"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error: %v", err)
	}

	if k1, ok := got["key1"]; k1 != nil || !ok {
		t.Errorf("GetOrSet() returned (%q, %t) for key1, expected (nil, true)", k1, ok)
	}
}

func TestUpdateManyNull(t *testing.T) {
	c := newCache()
	c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value"))
		return nil
	})

	c.SetIfExistMany(map[string][]byte{"key1": []byte("null")})

	got, err := c.GetOrSet(context.Background(), []string{"key1"}, func(key []string, set func(string, []byte)) error {
		set("key1", []byte("value that should not be fetched"))
		return nil
	})

	if err != nil {
		t.Errorf("GetOrSet() returned the unexpected error: %v", err)
	}

	if k1, ok := got["key1"]; k1 != nil || !ok {
		t.Errorf("GetOrSet() returned (%q, %t) for key1, expected (nil, true)", k1, ok)
	}
}
