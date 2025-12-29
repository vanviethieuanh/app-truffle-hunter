package main

import (
	"context"
	"sync"
)

func Mapper[I any, O any](
	ctx context.Context,
	inCh <-chan *I,
	buf int,
	mapFn func(*I) *O,
) <-chan *O {
	out := make(chan *O, buf)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case i, ok := <-inCh:
				if !ok {
					return
				}

				select {
				case out <- mapFn(i):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func Merge[T any](
	ctx context.Context,
	buf int,
	inCh ...<-chan *T,
) <-chan *T {
	out := make(chan *T, buf)

	var wg sync.WaitGroup
	wg.Add(len(inCh))

	for _, ch := range inCh {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func Filter[T any](
	ctx context.Context,
	inCh <-chan *T,
	pred func(*T) bool,
) <-chan *T {
	outCh := make(chan *T)

	go func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				return

			case v, ok := <-inCh:
				if !ok {
					return
				}

				if !pred(v) {
					continue
				}

				select {
				case outCh <- v:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return outCh
}

func Dumpster[T any](
	ctx context.Context,
	inCh <-chan *T,
) {
	go func() {
		for range inCh {
		}
	}()
}
