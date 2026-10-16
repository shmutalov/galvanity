package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/algorand/go-algorand-sdk/crypto"
)

func printHelp() {
	help := `
Usage: galvanity [flags] [search-type] <pattern>
search-type is matching function to search for pattern, it can be:
  exact    - search exact pattern (full address string)
  starts   - search address which starts with given pattern
  ends     - search address which ends with given pattern
  contains - search address which contains given pattern at any place (default)
pattern is correct **BASE32** hash string, alphabet is [A-Z] and [2-7]

Flags (must come before search-type):`

	fmt.Fprintln(flag.CommandLine.Output(), help)
	flag.PrintDefaults()
	fmt.Fprintln(flag.CommandLine.Output(), "\nGo to https://github.com/shmutalov/galvanity to download the latest source code")
}

// A backend searches until ctx is cancelled, sending matching accounts to
// found and adding the number of checked candidates to checked.
type backend interface {
	run(ctx context.Context, s search, found chan<- crypto.Account, checked *atomic.Uint64) error
}

const progressInterval = 5 * time.Second

func main() {
	backendName := flag.String("backend", "cpu", "search `backend`: cpu, or vulkan to use a GPU")
	device := flag.Int("device", -1, "Vulkan device `index` to use, see -list-devices; -1 picks the first usable discrete GPU, else integrated, virtual or CPU device")
	listDevices := flag.Bool("list-devices", false, "list Vulkan devices and exit")
	outFile := flag.String("out", "", "append found keys to `file` (owner-only permissions) and print only their addresses")
	gpuSettings := flag.String("gpu-config", "", "override the GPU `settings` as window/interleave/batch[/subgroup], e.g. 8/1/4 (default: chosen per device, see -list-devices)")
	flag.Usage = printHelp
	flag.Parse()
	gpuSettingsSet := false // also catches an explicit empty value
	flag.Visit(func(f *flag.Flag) { gpuSettingsSet = gpuSettingsSet || f.Name == "gpu-config" })

	if *listDevices {
		if err := printVulkanDevices(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	s, err := parseSearch(flag.Args())
	if err != nil {
		fmt.Println(err)
		printHelp()
		os.Exit(2)
	}

	var b backend
	switch *backendName {
	case "cpu":
		if gpuSettingsSet {
			fmt.Println("-gpu-config needs -backend vulkan")
			printHelp()
			os.Exit(2)
		}
		b = cpuBackend{}
	case "vulkan":
		vb := vulkanBackend{device: *device}
		if gpuSettingsSet {
			cfg, err := parseGPUConfig(*gpuSettings)
			if err != nil {
				fmt.Println(err)
				printHelp()
				os.Exit(2)
			}
			vb.config = &cfg
		}
		b = vb
	default:
		fmt.Printf("Invalid backend %q\n", *backendName)
		printHelp()
		os.Exit(2)
	}

	report := printAccount
	var writer *keyWriter
	var writeFailed <-chan error // stays nil (never ready) without -out
	if *outFile != "" {
		keys, err := openKeyFile(*outFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		writer = newKeyWriter(keys, *outFile)
		report, writeFailed = writer.add, writer.failed
	}

	fmt.Printf("Pattern to find: %s\n", s.pattern)
	fmt.Printf("Search type: %s\n", s.typ)
	fmt.Printf("Backend: %s\n", *backendName)
	if *outFile != "" {
		fmt.Printf("Saving found keys to: %s\n", *outFile)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Matching started...")

	found := make(chan crypto.Account, 64)
	done := make(chan error, 1)
	var checked atomic.Uint64
	go func() { done <- b.run(ctx, s, found, &checked) }()

	startedTime := time.Now()
	lastTime, lastChecked := startedTime, uint64(0)
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	for {
		select {
		case account := <-found:
			report(account)
		case err := <-writeFailed:
			// The writer prints the remaining keys in full; stop searching.
			fmt.Fprintln(os.Stderr, "Error:", err)
			stop()
		case now := <-ticker.C:
			total := checked.Load()
			speed := float64(total-lastChecked) / now.Sub(lastTime).Seconds()
			fmt.Printf("Processed: %d MH Speed: %s Time elapsed: %.2f s\n",
				total/1_000_000, formatSpeed(speed), now.Sub(startedTime).Seconds())
			lastTime, lastChecked = now, total
		case err := <-done:
			for len(found) > 0 {
				report(<-found)
			}
			if writer != nil {
				writer.close()
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
			// The search only ends on an error or an interrupt.
			os.Exit(1)
		}
	}
}

func formatSpeed(perSecond float64) string {
	if perSecond >= 1_000_000 {
		return fmt.Sprintf("%.2f MH/s", perSecond/1_000_000)
	}
	return fmt.Sprintf("%.2f KH/s", perSecond/1_000)
}
