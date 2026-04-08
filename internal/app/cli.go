package app

import "context"

type CLIRuntime struct {
	*Runtime
}

func NewCLIRuntime(ctx context.Context, opts Options) (*CLIRuntime, error) {
	runtime, err := New(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &CLIRuntime{Runtime: runtime}, nil
}
