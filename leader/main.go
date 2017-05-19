package main

import (
	"flag"

	"github.com/porpoises/kobun4/leader/supervisor"
)

var (
	nsjailPath = flag.String("nsjail_path", "", "Path to nsjail. Leave empty to not use nsjail. NOT RECOMMENDED!")
)

func main() {
	flag.Parse()

	f, err := supervisor.NewFollower(*nsjailPath, "/usr/local/bin/python3", []string{"-BIS", "-"}, []byte(`
import sys
sys.path.append('clients')

import k4client

k4 = k4client.Client()
print(k4.call('Doesnt.Exist', Nope='Nada'))

import urllib.request
urllib.request.urlopen('https://www.example.com').read()
`))
	if err != nil {
		panic(err)
	}

	if err := f.Start(); err != nil {
		panic(err)
	}

	f.Wait()
}
