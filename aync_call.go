package main

import (
	"context"
	"fmt"
	"golang.org/x/sync/semaphore"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	SleepTime = time.Duration(3000)
	TimeOut = time.Duration(300)
)

type Message struct {
	sn   int
	body []byte
}

func NewMessage(i int, b []byte) *Message {
	return &Message{sn: i, body: b}
}

type Actor struct {
	lock      *semaphore.Weighted
	cond      *sync.Cond
	condition bool
	Boxs      chan int
	v         int
	il        int32
}

func NewActor() *Actor {
	var lock sync.Mutex
	actor := &Actor{
		Boxs: make(chan int, 10),
		lock: semaphore.NewWeighted(int64(1)),
		cond: sync.NewCond(&lock),
		v:    0}
	return actor
}

func (actor *Actor) Lock(i int) {
	//lockNum := atomic.AddInt32(&actor.il, 1)
	//println(fmt.Sprintf("lockNum:%d i:%d lock", lockNum, i))
	ctx := context.Background()
	actor.lock.Acquire(ctx, 1)
}

func (actor *Actor) Unlock(i int) {
	//lockNum := atomic.AddInt32(&actor.il, -1)
	//println(fmt.Sprintf("lockNum:%d i:%d unlock", lockNum, i))
	actor.condition = true
	actor.lock.Release(1)
}

func (actor *Actor) Run() {
	go func() {
		for a := range actor.Boxs {
			actor.Recv(a)
		}
	}()
}

func (actor *Actor) Recv(a int) {
	actor.Lock(a)
	go func() {
		defer func() {
			actor.Unlock(a)
		}()

		actor.DoSomethings(a)
	}()
}

func (actor *Actor) DoSomethings(a int) {
	actor.v++
	println(fmt.Sprintf("---  v:%d a:%d time:%d", actor.v, a, time.Now().Second()))
	actor.asyncCall(func() {
		time.Sleep(time.Millisecond * SleepTime)
	})
	actor.v++
	println(fmt.Sprintf("+++  v:%d a:%d time:%d", actor.v, a, time.Now().Second()))
}

func (actor *Actor) asyncCall(f func()) {
	actor.Unlock(99)
	defer func() {
		actor.Lock(999)
		println(fmt.Sprintf("==== asynccall unlock:%v", true))
	}()

	c := make(chan bool)

	timeoutTimer := time.After(time.Millisecond * TimeOut)
	go func() {
		f()
		c <- true
	}()
	select {
	case <-timeoutTimer:
		return
	case <-c:
		return
	}
}

func waiting() {
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for {
		select {
		case _ = <-sigs:
			return
		}
	}
	fmt.Println("exiting")
}

func main() {
	actor := NewActor()
	for i := 0; i <= 10; i++ {
		go func(j int) {
			actor.Boxs <- j
		}(i)
	}
	actor.Run()
	waiting()
}
