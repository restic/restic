package chunker

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
)

func ExampleChunker() {
	// generate 32MiB of deterministic pseudo-random data
	data := getRandom(23, 32*1024*1024)

	// create a chunker
	chunker := New(bytes.NewReader(data), Pol(0x3DA3358B4DC173))

	// reuse this buffer
	buf := make([]byte, 8*1024*1024)

	for i := 0; i < 5; i++ {
		chunk, err := chunker.Next(buf)
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		fmt.Printf("%d %02x\n", chunk.Length, sha256.Sum256(chunk.Data))
	}

	// Output:
	// 2163460 4b94cb2cf293855ea43bf766731c74969b91aa6bf3c078719aabdd19860d590d
	// 643703 5727a63c0964f365ab8ed2ccf604912f2ea7be29759a2b53ede4d6841e397407
	// 1528956 a73759636a1e7a2758767791c69e81b69fb49236c6929e5d1b654e06e37674ba
	// 1955808 c955fb059409b25f07e5ae09defbbc2aadf117c97a3724e06ad4abd2787e6824
	// 2222372 6ba5e9f7e1b310722be3627716cf469be941f7f3e39a4c3bcefea492ec31ee56
}
