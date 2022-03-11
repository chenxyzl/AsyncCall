package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var maxGoNum = 10
var activeGoNum = 0

type Actor struct {
	lock sync.RWMutex
	Boxs chan int
	v    int
}

func (actor *Actor) Run() {
	go func() {
		for {
			select {
			case a := <-actor.Boxs:
				{
					actor.Recv(a)
				}
			}
		}
	}()
}

func (actor *Actor) Recv(a int) {
	actor.lock.Lock()
	for activeGoNum >= maxGoNum {
		runtime.Gosched()
	}
	activeGoNum++
	go func() {
		defer func() {
			activeGoNum--
			actor.lock.Unlock()
		}()
		actor.DoSomethings(a)
	}()
}

func (actor *Actor) DoSomethings(a int) {
	actor.v++
	println(fmt.Sprintf("---  v:%d a:%d time:%d", actor.v, a, time.Now().Second()))
	actor.asyncCall(func() {
		time.Sleep(time.Duration(3) * time.Second)
	})
	println(fmt.Sprintf("+++  v:%d a:%d time:%d", actor.v, a, time.Now().Second()))
}

func (actor *Actor) asyncCall(f func()) {
	actor.lock.Unlock()
	c := make(chan bool)
	go func() {
		f()
		c <- true
	}()
	select {
	case i := <-c:
		{
			println(fmt.Sprintf("==== asynccall recv:%d", i))
			break
		}
	}
	actor.lock.Lock()
}

func waiting() {
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	fmt.Println("awaiting signal")
	for {
		select {
		case _ = <-sigs:
			break
		}
	}
	fmt.Println("exiting")
}

func main() {
	fmt.Println("1")
	a := make(chan bool)
	go func() {
		time.Sleep(2 * time.Second)
		a <- true
	}()
	b := <-a
	fmt.Println("2")
	println(b)
	actor := &Actor{Boxs: make(chan int, 10), v: 0}
	for i := 0; i <= 10; i++ {
		go func(j int) {
			actor.Boxs <- j
		}(i)
	}
	actor.Run()
	waiting()
}
