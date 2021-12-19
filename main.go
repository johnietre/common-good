package main

import (
  "flag"
  "log"
  "os"
)

var logger = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

func main() {
  addrPtr := flag.String("addr", "localhost:9876", "Address to run server on")
  flag.Parse()

  logger.Fatal(StartServer(*addrPtr))
}
