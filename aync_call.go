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
	SleepTime        = time.Duration(100)
	AsyncTimeOut     = time.Duration(1000)
	MailBoxSize      = 10
	ActorCount       = 100000 //同时运行多少个actor
	ActorPerMsgCount = 10  //每个actor多少条消息
)

//最大Rpc协程数[包含挂起]-1台机器通常可以轻松运行一百万个协程，这里设置保守设置来,[虽然go的实际并发为runtime.GOMAXPROCS()]
const MaxGoCount = 1024 * 10

//运行中的的Rpc协程数
var RunningGoNum int32 = 0

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
	lock         *semaphore.Weighted
	cond         *sync.Cond
	condition    bool
	Boxs         chan int
	v            int
	il           int32
	clientLastSn int32 //(客户端消息的sn，需要排序挨个处理)
}

func NewActor() *Actor {
	var lock sync.Mutex
	actor := &Actor{
		Boxs: make(chan int, MailBoxSize),
		lock: semaphore.NewWeighted(int64(1)),
		cond: sync.NewCond(&lock),
		v:    0}
	return actor
}

func (actor *Actor) Lock(i int) {
	ctx := context.Background()
	actor.lock.Acquire(ctx, 1)
}

func (actor *Actor) Unlock(i int) {
	actor.condition = true
	actor.lock.Release(1)
}

func (actor *Actor) Run() {
	go func() {
		for a := range actor.Boxs {
			//todo 区分客户端消息和内部rpc
			actor.Recv(a)
		}
	}()
}

var goNumLock sync.Mutex

func (actor *Actor) Recv(a int) {
	goNumLock.Lock()
	for {
		if RunningGoNum >= MaxGoCount {
			runtime.Gosched()
		} else {
			break
		}
	}
	goNumLock.Unlock()
	actor.Lock(a)
	atomic.AddInt32(&RunningGoNum, 1)
	go func() {
		defer func() {
			atomic.AddInt32(&RunningGoNum, -1)
			actor.Unlock(a)
		}()

		actor.DoSomethings(a)
	}()
}

func (actor *Actor) DoSomethings(a int) {
	actor.v++
	//fmt.Printf("---  v:%d a:%d time:%d\n", actor.v, a, time.Now().Second())
	actor.asyncCall(func() {
		time.Sleep(time.Millisecond * SleepTime)
	})
	actor.v++

	atomic.AddInt32(&currentMessageCount, 1)
	//fmt.Printf("+++  v:%d a:%d time:%d\n", actor.v, a, time.Now().Second())
	if currentMessageCount == totalMessageCount {
		fmt.Println("do message size end:", currentMessageCount)
	}
}

func (actor *Actor) asyncCall(f func()) {
	actor.Unlock(99)
	defer func() {
		actor.Lock(999)
		//fmt.Printf("==== asynccall unlock:%v\n", true)
	}()

	c := make(chan bool)

	timeoutTimer := time.After(time.Millisecond * AsyncTimeOut)
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
	for m := 0; m <= ActorCount; m++ {
		actor := NewActor()
		for i := 0; i < ActorPerMsgCount; i++ {
			go func(j int) {
				actor.Boxs <- j
			}(i)
		}
		actor.Run()
	}
	waiting()
}
