package main

import (
  "errors"
  "io"
  "net/http"
  "time"

  webs "golang.org/x/net/websocket"
)

const pagePath = "./index.html"

// map[Room.RoomId]*Room
var rooms map[string]*Room

func StartServer(addr string) error {
  s := &http.Server{
    Addr: addr,
    Handler: routes(),
    ErrorLog: logger,
  }
  logger.Printf("starting server on %s", addr)
  return s.ListenAndServe()
}

func routes() *http.ServeMux {
  r := http.NewServeMux()
  r.HandleFunc("/", pageHandler)
  r.Handle("/ws", webs.Handler(wsHandler))
  return r
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
  http.ServeFile(w, r, pagePath)
}

func wsHandler(ws *webs.Conn) {
  addr := ws.Request().RemoteAddr
  logm := func(format string, args ...interface{}) {
    logger.Printf(addr + ": " + format, args...)
  }
  defer func() {
    logm("closing connection")
    if err := ws.Close(); err != nil {
      logm("error closing connection: %v", err)
    }
  }()
  logm("opened connection")
  msg := Message{
    Action: ActionChat,
    Sender: "server",
  }
  go func() {
    var msg Message
    if err := webs.JSON.Receive(ws, &msg); err != nil {
      if errors.Is(err, io.EOF) {
        return
      }
      logm("error: %v", err)
      return
    } else {
      logm("%s => %s", msg.Sender, msg.Contents)
    }
  }()
  t := time.NewTicker(time.Second * 5)
  defer t.Stop()
  for {
    <-t.C
    msg.Contents = time.Now().Format(time.Stamp)
    if err := webs.JSON.Send(ws, msg); err != nil {
      if errors.Is(err, io.EOF) {
        return
      }
      logm("error: %v", err)
      return
    }
  }
}
