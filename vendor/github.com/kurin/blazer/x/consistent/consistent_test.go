package consistent

import (
	"context"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/kurin/blazer/b2"
)

const (
	apiID      = "B2_ACCOUNT_ID"
	apiKey     = "B2_SECRET_KEY"
	bucketName = "consistobucket"
)

func TestOperationLive(t *testing.T) {
	ctx := context.Background()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	g := NewGroup(bucket, "tester")
	name := "some_kinda_name/thing.txt"

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			var n int
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := g.Operate(ctx, name, func(b []byte) ([]byte, error) {
					if len(b) > 0 {
						i, err := strconv.Atoi(string(b))
						if err != nil {
							return nil, err
						}
						n = i
					}
					return []byte(strconv.Itoa(n + 1)), nil
				}); err != nil {
					t.Error(err)
				}
				t.Logf("thread %d: successful %d++", i, n)
			}
		}()
	}
	wg.Wait()

	r, err := g.NewReader(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if n != 100 {
		t.Errorf("result: got %d, want 100", n)
	}
}

type jsonThing struct {
	Boop   int `json:"boop_field"`
	Thread int `json:"thread_id"`
}

func TestOperationJSONLive(t *testing.T) {
	ctx := context.Background()
	bucket, done := startLiveTest(ctx, t)
	defer done()

	g := NewGroup(bucket, "tester")
	name := "some_kinda_json/thing.json"

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		i := i
		go func() {
			var n int
			defer wg.Done()
			for j := 0; j < 4; j++ {
				// Pass both a struct and a pointer to a struct.
				var face interface{}
				face = jsonThing{}
				if j%2 == 0 {
					face = &jsonThing{}
				}
				if err := g.OperateJSON(ctx, name, face, func(j interface{}) (interface{}, error) {
					jt := j.(*jsonThing)
					n = jt.Boop
					return &jsonThing{
						Boop:   jt.Boop + 1,
						Thread: i,
					}, nil
				}); err != nil {
					t.Error(err)
				}
				t.Logf("thread %d: successful %d++", i, n)
			}
		}()
	}
	wg.Wait()

	if err := g.OperateJSON(ctx, name, &jsonThing{}, func(i interface{}) (interface{}, error) {
		jt := i.(*jsonThing)
		if jt.Boop != 16 {
			t.Errorf("got %d boops; want 16", jt.Boop)
		}
		return nil, nil
	}); err != nil {
		t.Error(err)
	}
}

func startLiveTest(ctx context.Context, t *testing.T) (*b2.Bucket, func()) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
		return nil, nil
	}
	client, err := b2.NewClient(ctx, id, key)
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	bucket, err := client.NewBucket(ctx, id+"-"+bucketName, nil)
	if err != nil {
		t.Fatal(err)
		return nil, nil
	}
	f := func() {
		iter := bucket.List(ctx, b2.ListHidden())
		for iter.Next() {
			if err := iter.Object().Delete(ctx); err != nil {
				t.Error(err)
			}
		}
		if err := iter.Err(); err != nil && !b2.IsNotExist(err) {
			t.Error(err)
		}
		if err := bucket.Delete(ctx); err != nil && !b2.IsNotExist(err) {
			t.Error(err)
		}
	}
	return bucket, f
}

type object struct {
	o   *b2.Object
	err error
}
