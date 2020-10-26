package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/coyove/script"
)

const VERSION = "0.2.0"

var (
	goroutinePerCPU = flag.Int("goroutine", 2, "goroutines per CPU")
	output          = flag.String("o", "none", "separated by comma: (none|compileonly|opcode|bytes|ret|timing)+")
	input           = flag.String("i", "f", "input source, 'f': file, '-': stdin, others: string")
	version         = flag.Bool("v", false, "print version and usage")
	quiet           = flag.Bool("quieterr", false, "suppress the error output (if any)")
	timeout         = flag.Int("t", 0, "max execution time in ms")
	apiServer       = flag.String("serve", "", "start as language playground")
)

func main() {
	source := ""
	for i, arg := range os.Args {
		if _, err := os.Stat(arg); err == nil && i > 0 {
			source = arg
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			break
		}
	}

	flag.Parse()

	if *apiServer != "" {
		script.RemoveGlobalValue("sleep")
		script.RemoveGlobalValue("narray")
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			c := strings.TrimSpace(r.FormValue("code"))
			if c == "" {
				c = strings.TrimSpace(r.URL.Query().Get("code"))
			}
			if len(c) > 16*1024 {
				c = c[:16*1024]
			}

			p, err := script.LoadString(c)
			if err != nil {
				writeJSON(w, map[string]interface{}{"error": err.Error()})
				return
			}
			bufOut := &bytes.Buffer{}
			p.SetTimeout(time.Second / 2)
			p.MaxCallStackSize = 100
			p.MaxStackSize = 32 * 1024
			p.Stdout = bufOut
			v, v1, err := p.Run()
			if err != nil {
				writeJSON(w, map[string]interface{}{
					"error":  err.Error(),
					"opcode": p.PrettyCode(),
				})
				return
			}
			results := make([]interface{}, 1+len(v1))
			results[0] = v.Interface()
			for i := range v1 {
				results[1+i] = v1[i].Interface()
			}
			writeJSON(w, map[string]interface{}{
				"elapsed": time.Since(start).Seconds(),
				"results": results,
				"stdout":  bufOut.String(),
				"opcode":  p.PrettyCode(),
			})
		})
		log.Println("listen", *apiServer)
		http.ListenAndServe(*apiServer, nil)
		return
	}

	log.SetFlags(0)

	if *version {
		fmt.Println("\"script\": script virtual machine v" + VERSION + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")")
		flag.Usage()
		return
	}

	{
		f, err := os.Create("cpuprofile")
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	switch *input {
	case "f":
		if source == "" {
			log.Fatalln("Please specify the input file: ./script <filename>")
		}
	case "-":
		buf, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalln(err)
		}
		source = string(buf)
	default:
		if _, err := os.Stat(*input); err == nil {
			source = *input
			*input = "f"
		} else {
			source = *input
		}
	}

	var _opcode, _timing, _ret, _compileonly bool

ARG:
	for _, a := range strings.Split(*output, ",") {
		switch a {
		case "n", "no", "none":
			_opcode, _ret, _timing, _compileonly = false, false, false, false
			break ARG
		case "o", "opcode", "op":
			_opcode = true
		case "r", "ret", "return":
			_ret = true
		case "t", "time", "timing":
			_timing = true
		case "co", "compile", "compileonly":
			_compileonly = true
		}
	}

	runtime.GOMAXPROCS(runtime.NumCPU() * *goroutinePerCPU)
	start := time.Now()

	var b *script.Program
	var err error

	defer func() {
		if *quiet {
			recover()
		}

		if _opcode {
			log.Println(b.PrettyCode())
		}
		if _timing {
			e := float64(time.Now().Sub(start).Nanoseconds()) / 1e6
			if e < 1000 {
				log.Printf("Time elapsed: %.1fms\n", e)
			} else {
				log.Printf("Time elapsed: %.3fs\n", e/1e3)
			}
		}
	}()

	if *input == "f" {
		b, err = script.LoadFile(source)
	} else {
		b, err = script.LoadString(source)
	}
	if err != nil {
		log.Fatalln(err)
	}

	if _compileonly {
		return
	}

	if *timeout > 0 {
		b.SetTimeout(time.Second * time.Duration(*timeout))
	}

	i, i2, err := b.Call()
	if _ret {
		fmt.Print(i)
		for _, a := range i2 {
			fmt.Print(" ", a)
		}
		fmt.Print(" ", err, "\n")
	}
}

func writeJSON(w http.ResponseWriter, m map[string]interface{}) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	buf, _ := json.Marshal(m)
	w.Write(buf)
}