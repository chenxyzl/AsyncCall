package main

import (
	"context"
	"fmt"
	"golang.org/x/sync/semaphore"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	SleepTime             = time.Duration(100) * time.Millisecond
	AsyncTimeOut          = time.Duration(1000) * time.Millisecond
	MailBoxSize           = 10
	ActorCount            = 100000 //同时运行多少个actor
	ActorPerMsgCount      = 10     //每个actor多少条消息
	ActorMaxRunningGoSize = 10     //单个actor内最多同时处理消息的go程的数量
)

const totalMessageCount int32 = ActorCount * ActorPerMsgCount

var currentMessageCount int32 = 0

type Message struct {
	sn   int
	body []byte
}

func NewMessage(i int, b []byte) *Message {
	return &Message{sn: i, body: b}
}

type Actor struct {
	lock             *semaphore.Weighted
	goNumLock        sync.Mutex
	cond             *sync.Cond
	condition        bool
	Boxs             chan int
	v                int
	il               int32
	clientLastSn     int32 //(客户端消息的sn，需要排序挨个处理)
	maxRunningGoSize int32 //让box的size等于同时运行的go的数量的大小。go程再多也没有啥意义了 size等于1就等同于单线程了
	runningGoNum     int32 //最大值必须小于等于Boxs的缓存大小
}

func NewActor(boxSize int32, maxRunningGoSize int32) *Actor {
	if boxSize <= 0 {
		panic("boxSize must bigger than 0")
	}
	actor := &Actor{
		Boxs:             make(chan int, boxSize),
		maxRunningGoSize: maxRunningGoSize,
		lock:             semaphore.NewWeighted(int64(1)),
		v:                0}
	return actor
}

func (actor *Actor) Lock(i int) {
	ctx := context.Background()
	actor.lock.Acquire(ctx, 1)
	//fmt.Printf("lock  v:%d a:%d time:%d\n", actor.v, i, time.Now().Second())
}

func (actor *Actor) Unlock(i int) {
	actor.condition = true
	//fmt.Printf("unlock  v:%d a:%d time:%d\n", actor.v, i, time.Now().Second())
	actor.lock.Release(1)
}

func (actor *Actor) Run() {
	go func() {
		for a := range actor.Boxs {
			_ = a
			//todo 区分客户端消息和内部rpc
			actor.Recv(a)
		}
	}()
}

func (actor *Actor) Recv(a int) {
	actor.goNumLock.Lock()
	for {
		if actor.runningGoNum >= actor.maxRunningGoSize {
			runtime.Gosched()
		} else {
			break
		}
	}
	actor.goNumLock.Unlock()
	actor.Lock(a)
	atomic.AddInt32(&actor.runningGoNum, 1)
	go func() {
		defer func() {
			atomic.AddInt32(&actor.runningGoNum, -1)
			actor.Unlock(a)
			if currentMessageCount == totalMessageCount {
				fmt.Println("all message complete, count:", currentMessageCount)
			}
		}()

		actor.DoSomethings(a)
	}()
}

func (actor *Actor) DoSomethings(a int) {
	actor.v++
	//fmt.Printf("---  v:%d a:%d time:%d\n", actor.v, a, time.Now().Second())
	actor.asyncCall(a, func(int) {
		time.Sleep(SleepTime)
	})
	actor.v++

	atomic.AddInt32(&currentMessageCount, 1)
	//fmt.Printf("+++  v:%d a:%d time:%d\n", actor.v, a, time.Now().Second())
}

func (actor *Actor) asyncCall(i int, f func(int)) {
	actor.Unlock(i)
	defer func(a int) {
		actor.Lock(a)
		//fmt.Printf("==== asynccall unlock:%v\n", true)
	}(i)

	c := make(chan bool)

	timeoutTimer := time.After(AsyncTimeOut)
	go func() {
		f(i)
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
	for m := 0; m < ActorCount; m++ {
		actor := NewActor(MailBoxSize, ActorMaxRunningGoSize)
		actor.Run()
		for i := 0; i < ActorPerMsgCount; i++ {
			actor.Boxs <- i
		}
	}
	waiting()
}
