package main

import (
  "fmt"
  "github.com/google/uuid"
)

func main() {
  bid := uuid.New().String()
  fmt.Printf("%s\n", bid)
}
