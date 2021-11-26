package server

import (
	"sync"
	"time"

	"github.com/pkg/errors"
)

type Data interface{}

type Result struct {
	Data  Data
	Error error
}

// THREAD SAFE
type Future struct {
	channel chan Result
	m       sync.Mutex // protects n and result
	n       int
	result  *Result
}

func NewFuture() *Future {
	return &Future{channel: make(chan Result)}
}

func NewFutureResolved(result Result) *Future {
	return &Future{result: &result}
}

func (f *Future) Resolve(result Result) *Future {
	f.m.Lock()
	defer f.m.Unlock()
	f.result = &result
	n := f.n

	for i := 0; i < n; i++ {
		f.channel <- result
	}
	return f
}

func (f *Future) Await() Result {
	f.m.Lock()
	result := f.result
	if result != nil {
		f.m.Unlock()
		return *result
	} else {
		f.n++
		f.m.Unlock() // unlock here before waiting on the channel
		return <-f.channel
	}
}

var TimeoutError = errors.New("timeout waiting for future")

func (f *Future) AwaitTimeout(d time.Duration) Result {
	f.m.Lock()
	result := f.result
	if result != nil {
		f.m.Unlock()
		return *result
	} else {
		f.n++
		f.m.Unlock() // unlock here before waiting on the channel
		select {
		case res2 := <-f.channel:
			return res2
		case <-time.After(d):
			f.m.Lock()
			defer f.m.Unlock()
			f.n--
			return Result{Error: TimeoutError}
		}
	}
}

type SmartPerformer interface {
	Get(key string) Data
	Put(key string, data Data)
	Perform(key string) Result
}

// THREAD SAFE
type futureMap struct {
	m       sync.Mutex // protects futures
	futures map[string]*Future
}

func newFutureMap() *futureMap {
	return &futureMap{futures: map[string]*Future{}}
}

func (f *futureMap) getOrCreate(key string) (_future *Future, isNew bool) {
	f.m.Lock()
	defer f.m.Unlock()
	future, ok := f.futures[key]
	if ok {
		return future, false
	}
	future = NewFuture()
	f.futures[key] = future
	return future, true
}

func (f *futureMap) finish(key string, result Result) {
	f.m.Lock()
	defer f.m.Unlock()
	future := f.futures[key]
	future.Resolve(result)

	// TODO remove from futureMap, should be cached in db
	// delete(f.futures, key)
}

// THREAD SAFE, because all the fields are thread safe
type SmartWorkPool struct {
	performer SmartPerformer
	workQueue chan string
	futureMap *futureMap
}

func NewSmartWorkPool(performer SmartPerformer) *SmartWorkPool {
	return &SmartWorkPool{
		performer: performer,
		workQueue: make(chan string),
		futureMap: newFutureMap(),
	}
}

var databaseDisabled = false // for debugging

func (s *SmartWorkPool) work(i int) {
	for key := range s.workQueue {
		result := s.performer.Perform(key)
		if result.Error == nil && !databaseDisabled {
			s.performer.Put(key, result.Data)
		}
		s.futureMap.finish(key, result)
	}
}

func (s *SmartWorkPool) ProcessKey(key string) *Future {
	if !databaseDisabled {
		data := s.performer.Get(key)
		if data != nil {
			return NewFutureResolved(Result{Data: data})
		}
	}
	future, isNew := s.futureMap.getOrCreate(key)
	if isNew {
		s.workQueue <- key
	}
	return future
}

func (s *SmartWorkPool) Start(n int) {
	for i := 0; i < n; i++ {
		go s.work(i)
	}
}
