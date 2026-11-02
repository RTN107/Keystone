// Command embedcheck makes live calls to the embeddings service and prints what
// it actually returns: the model's default vector size, the size when one is
// requested, the vector length (norm), and a feel for similarity scores.
//
// It exists because the vector column size is a one-way door: confirm the real
// number here before any migration depends on it. It never prints the key.
//
//	VOYAGE_API_KEY=... go run ./cmd/embedcheck
//
// VOYAGE_BASE_URL and VOYAGE_MODEL are honoured, so a different endpoint or model can be
// checked before anything depends on it.
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"keystone/internal/embed"
)

// pause waits between live calls when EMBEDCHECK_PAUSE_SECONDS is set. An account
// without a payment method is limited to 3 requests a minute, so 21 keeps a run
// under it. The default is no pause.
func pause() {
	if n, err := strconv.Atoi(os.Getenv("EMBEDCHECK_PAUSE_SECONDS")); err == nil && n > 0 {
		time.Sleep(time.Duration(n) * time.Second)
	}
}

// modelName is VOYAGE_MODEL when set, so you can check a model before using it, and the
// default model otherwise.
func modelName() string {
	if m := os.Getenv("VOYAGE_MODEL"); m != "" {
		return m
	}
	return embed.Model
}

func newClient(base, key string, dimension int) *embed.Client {
	return embed.New(base, key, dimension).WithModel(modelName())
}

func main() {
	key := os.Getenv("VOYAGE_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "VOYAGE_API_KEY is not set")
		os.Exit(2)
	}
	base := os.Getenv("VOYAGE_BASE_URL")
	if base == "" {
		base = embed.DefaultBaseURL
	}
	fmt.Printf("endpoint: %s\nmodel:    %s\n\n", newClient(base, key, 0).Endpoint(), modelName())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	texts := []string{
		"Employees receive 24 days of paid annual leave each year.",
		"A priority-one ticket breaches its SLA when it is not acknowledged within 15 minutes.",
		"Clients who commit to a twelve month retainer can be offered up to 10% off the monthly fee.",
	}
	question := "How many days of holiday do I get?"

	// 1. No size requested: what does the model return by default?
	def, err := newClient(base, key, 0).Embed(ctx, texts, embed.Document)
	check(err)
	pause()
	fmt.Printf("default size (no output_dimension sent): %d\n", len(def.Vectors[0]))

	// 2. Each documented size, requested explicitly: is the parameter honoured?
	for _, d := range []int{256, 512, 1024, 2048} {
		pause()
		res, err := newClient(base, key, 0).EmbedWithDimension(ctx, texts[:1], embed.Document, d)
		var de *embed.DimensionError
		switch {
		case errors.As(err, &de):
			fmt.Printf("output_dimension=%-4d requested, but returned: %d\n", d, de.Got)
		case err != nil:
			fmt.Printf("output_dimension=%-4d error: %v\n", d, err)
		default:
			fmt.Printf("output_dimension=%-4d returned: %d\n", d, len(res.Vectors[0]))
		}
	}

	// 3. Pinned at 1024, the way the API will call it: vector length and scores.
	c := newClient(base, key, 1024)
	pause()
	docs, err := c.Embed(ctx, texts, embed.Document)
	check(err)
	pause()
	q, err := c.Embed(ctx, []string{question}, embed.Query)
	check(err)

	fmt.Printf("\nvector length (norm) of a 1024 vector: %.4f\n", norm(docs.Vectors[0]))
	fmt.Printf("tokens used by the three documents: %d\n\n", docs.Tokens)
	fmt.Printf("query: %q\n", question)
	for i, t := range texts {
		fmt.Printf("  cosine similarity %.4f  %q\n", dot(q.Vectors[0], docs.Vectors[i]), t)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAILED:", err)
		os.Exit(1)
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func norm(a []float32) float64 { return math.Sqrt(dot(a, a)) }
