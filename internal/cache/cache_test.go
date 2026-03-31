package cache

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to be found")
	}
	if got != "value1" {
		t.Errorf("got %q, want value1", got)
	}
}

func TestCache_GetMissing(t *testing.T) {
	c := New[string](5 * time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected missing key to return false")
	}
}

func TestCache_Expiry(t *testing.T) {
	c := New[string](1 * time.Millisecond)

	c.Set("key1", "value1")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected expired key to return false")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")
	c.Invalidate("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected invalidated key to return false")
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")
	c.InvalidateAll()

	if c.Len() != 0 {
		t.Errorf("expected empty cache after InvalidateAll, got %d entries", c.Len())
	}
}

func TestCache_Len(t *testing.T) {
	c := New[int](5 * time.Minute)

	if c.Len() != 0 {
		t.Errorf("new cache Len = %d, want 0", c.Len())
	}

	c.Set("a", 1)
	c.Set("b", 2)

	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestCache_OverwriteKey(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "old")
	c.Set("key1", "new")

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to be found")
	}
	if got != "new" {
		t.Errorf("got %q, want new", got)
	}
}

type testStruct struct {
	Name  string
	Count int
}

func TestCache_StructValues(t *testing.T) {
	c := New[testStruct](5 * time.Minute)

	c.Set("s1", testStruct{Name: "boulder", Count: 42})

	got, ok := c.Get("s1")
	if !ok {
		t.Fatal("expected s1 to be found")
	}
	if got.Name != "boulder" || got.Count != 42 {
		t.Errorf("got %+v, want {boulder 42}", got)
	}
}
