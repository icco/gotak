# sanic

[![GoDoc](https://godoc.org/github.com/ifo/sanic?status.svg)](https://godoc.org/github.com/ifo/sanic)

sanic is a clone of [Twitter snowflake](https://github.com/twitter/snowflake/tree/snowflake-2010) (the 2010 version), written in [Golang](https://golang.org/). More specifically, the [IdWorker section of snowflake](https://github.com/twitter/snowflake/blob/snowflake-2010/src/main/scala/com/twitter/service/snowflake/IdWorker.scala).

### Usage

To use sanic, either make a new worker, or select a premade one:

```go
package main

import (
	"fmt"

	"github.com/ifo/sanic"
)

func main() {
	worker := sanic.NewWorker7()
	// equivalent to:
	// worker := sanic.NewWorker(0, 1451606400, 0, 10, 31, time.Second)

	id := worker.NextID()
	idString := worker.IDString(id)
	fmt.Println(id)       // e.g. 5292179457
	fmt.Println(idString) // e.g. "AUBwOwE"
}
```

Check out [the examples](https://github.com/ifo/sanic/tree/master/examples) for
more.

### Future Improvements

Many things are missing from sanic, so here's a TODO list of some of those
things:

- **Tests**: sanic has no tests.
This is probably the next thing I should do.
- **More Random Looking IDs**: Generated IDs don't change that much.
Some creative bit manipulation could likely change this while still ensuring
that IDs are unique (so long as you use the same bit manipulation each time).

## License

sanic is ISC licensed.
Check out the [LICENSE](https://github.com/ifo/sanic/blob/master/LICENSE) file.
