package task

import (
	"context"
	"shortURL/internal/repository"
)

type IncrementTask struct {
	Repo repository.URLRepo
	Code string
	Ctx  context.Context
}

func (t *IncrementTask) Execute() error {
	return t.Repo.IncrementClicks(t.Ctx, t.Code)
}
