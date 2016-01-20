package testdata

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	VarnishURL string
	Version    string
	VersionRE  = regexp.MustCompile(`varnish-(\d\.\d\.\d)`)
)

func request(tb testing.TB, method, path string, code int) {
	req, err := http.NewRequest(method, VarnishURL+path, nil)
	if err != nil {
		tb.Fatal(err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatal(err)
	}

	if res.StatusCode != code {
		tb.Fatalf("Expected code %d, got %d", code, res.StatusCode)
	}
}

func TestScript(t *testing.T) {
	request(t, "GET", "/200", 200)
	request(t, "GET", "/404", 404)
	request(t, "GET", "/500", 500)
}

func BenchmarkScript(b *testing.B) {
	for i := 0; i < b.N; i++ {
		request(b, "GET", "/200", 200)
		request(b, "GET", "/404", 404)
		request(b, "GET", "/500", 500)
	}
}

func assert(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nPANIC: %v\n\n", err)
		panic(err)
	}
}

func TestMain(m *testing.M) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "This tool is used to generate Varnish logs for unit tests.\n")
		fmt.Fprintf(os.Stderr, "It starts own backend, varnishd, varnishlog, and then makes some requests.\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	exitCode := 255
	defer func() { os.Exit(exitCode) }()

	// start backend
	backend := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		fmt.Fprint(os.Stderr, req.RequestURI)

		p := strings.Split(req.RequestURI, "/")
		code, err := strconv.Atoi(p[len(p)-1])
		assert(err)

		fmt.Fprintf(os.Stderr, " -> %d\n", code)
		rw.WriteHeader(code)
	}))
	fmt.Fprintf(os.Stderr, "Backend started at %s\n", backend.URL)
	defer backend.Close()

	// get random unused port for Varnish
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert(err)
	addr := l.Addr().String()
	err = l.Close()
	assert(err)
	VarnishURL = "http://" + addr

	// get random temporary directory
	var b []byte
	tmp, err := ioutil.TempDir("", "varnish-")
	assert(err)
	err = os.Chmod(tmp, 0777)
	assert(err)
	defer func() {
		fmt.Fprintf(os.Stderr, "removing %s\n", tmp)
		err = os.RemoveAll(tmp)
		assert(err)
		fmt.Fprintf(os.Stderr, "removed\n")
	}()

	// check varnishd version
	path, err := exec.LookPath("varnishd")
	assert(err)
	b, err = exec.Command(path, "-V").CombinedOutput()
	assert(err)
	subm := VersionRE.FindStringSubmatch(string(b))
	fmt.Fprintf(os.Stderr, "%s", b)
	if len(subm) < 2 {
		panic("failed to parse version")
	}
	Version = subm[1]
	fmt.Fprintf(os.Stderr, "Parsed version: %s\n", Version)

	// start varnishd
	args := []string{"-n", tmp, "-a", addr, "-b", strings.TrimPrefix(backend.URL, "http://"), "-F"}
	{
		cmd := exec.Command(path, args...)
		fmt.Fprintf(os.Stderr, "Running %q\n", strings.Join(cmd.Args, " "))
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err = cmd.Start()
		assert(err)
		defer func() {
			fmt.Fprintf(os.Stderr, "stopping varnishd...\n")
			err = cmd.Process.Kill()
			assert(err)
			cmd.Wait()
			fmt.Fprintf(os.Stderr, "varnishd stopped, output:\n%s\n", out.Bytes())
		}()
	}

	time.Sleep(time.Second)
	request(nil, "GET", "/startup/417", 417)

	// start varnishlogs
	path, err = exec.LookPath("varnishlog")
	assert(err)
	for _, group := range []string{"session", "request", "vxid", ""} {
		args = []string{"-N", filepath.Join(tmp, "_.vsm")}
		filename := Version
		if group != "" {
			args = append(args, "-g", group)
			filename += "-" + group
		}
		filename += ".log"

		cmd2 := exec.Command(path, args...)
		fmt.Fprintf(os.Stderr, "Running %q, logging to %s\n", strings.Join(cmd2.Args, " "), filename)
		f2, err := os.Create(filename)
		assert(err)
		defer f2.Close()
		cmd2.Stdout = f2
		var out2 bytes.Buffer
		cmd2.Stderr = &out2
		err = cmd2.Start()
		assert(err)
		defer func() {
			fmt.Fprintf(os.Stderr, "stopping varnishlog...\n")
			err = cmd2.Process.Kill()
			assert(err)
			cmd2.Wait()
			fmt.Fprintf(os.Stderr, "varnishlog stopped, output:\n%s\n", out2.Bytes())
		}()
	}

	// run tests, with small delay before and after
	time.Sleep(10 * time.Second)
	defer time.Sleep(10 * time.Second)
	exitCode = m.Run()
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()
}
