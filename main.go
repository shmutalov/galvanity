package main

import (
	"fmt"
	"github.com/algorand/go-algorand-sdk/crypto"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	fmt.Printf("CPU: %d\n", runtime.NumCPU())

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	fmt.Println("Crunching started...")

	startedTime := time.Now()
	counter := make(chan uint64)
	for i := 0; i < runtime.NumCPU()-1; i++ {
		address := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
		go func(addr string) {
			var i uint64
			for {
				account := crypto.GenerateAccount()
				// if account.Address.String() == addr {
				if strings.HasPrefix(account.Address.String(), "AAAAA") {
					fmt.Printf("FOUND ADDR: %s\nPUB: %v\nPK: %v\n", account.Address, account.PublicKey, account.PrivateKey)
					c <- os.Interrupt
					return
				}

				if i%100 == 0 {
					counter <- i
				}

				i++
				runtime.Gosched()
			}
		}(address)
	}

	var total uint64
	var oldTotal uint64
	oldTime := startedTime
	for {
		select {
		case x := <-counter:
			total += x
			if total%1_000_000 == 0 {
				now := time.Now()
				speed := float64(total-oldTotal) / now.Sub(oldTime).Seconds()

				fmt.Printf("PARSED: %d SPEED %d/s ELAPSED: %f s\n", total, uint64(speed), now.Sub(startedTime).Seconds())
				oldTotal = total
				oldTime = time.Now()
			}
		}

		runtime.Gosched()
	}
}
