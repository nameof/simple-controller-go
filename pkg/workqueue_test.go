package main

import (
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	queue := workqueue.New()

	queue.Add("A")
	queue.Get()
	queue.Done("A")
}

func TestDelayingQueue(t *testing.T) {
	queue := workqueue.NewDelayingQueue()
	defer queue.ShutDown()

	queue.AddAfter("A", time.Second*3)

	if queue.Len() != 0 {
		t.Errorf("delay failed")
		return
	}

	time.Sleep(time.Second * 4)

	if queue.Len() != 1 {
		t.Errorf("delay failed")
	}
}

func TestRateLimitingQueue(t *testing.T) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	defer queue.ShutDown()

	value := "A"
	queue.Add(value)
	queue.AddRateLimited(value)

	if queue.NumRequeues(value) != 1 {
		t.Errorf("limit failed")
		return
	}

	queue.Forget(value)

	if queue.NumRequeues(value) != 0 {
		t.Errorf("limit failed")
	}
}

func TestLimiter(t *testing.T) {
	limiter := rate.NewLimiter(rate.Limit(10), 100)
	limiter.AllowN(time.Now(), 100)
}
