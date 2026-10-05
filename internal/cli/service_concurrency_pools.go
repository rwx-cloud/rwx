package cli

import "github.com/rwx-cloud/rwx/internal/api"

type ListConcurrencyPoolsConfig struct {
	Search string
	Limit  int
	Cursor string
}

type ShowConcurrencyPoolConfig struct {
	PoolID string
	Limit  int
	Cursor string
}

type ShowConcurrencyPoolResult struct {
	ConcurrencyPool api.ConcurrencyPool
	Queue           []api.ConcurrencyPoolLease
	Pagination      api.ConcurrencyPoolPagination
}

func (s Service) ListConcurrencyPools(cfg ListConcurrencyPoolsConfig) (*api.ListConcurrencyPoolsResult, error) {
	return s.APIClient.ListConcurrencyPools(api.ListConcurrencyPoolsConfig{
		Search: cfg.Search,
		Limit:  cfg.Limit,
		Cursor: cfg.Cursor,
	})
}

func (s Service) ShowConcurrencyPool(cfg ShowConcurrencyPoolConfig) (*ShowConcurrencyPoolResult, error) {
	pool, err := s.APIClient.ShowConcurrencyPool(cfg.PoolID)
	if err != nil {
		return nil, err
	}
	queue, err := s.APIClient.ListConcurrencyPoolQueue(api.ListConcurrencyPoolQueueConfig{
		PoolID: cfg.PoolID,
		Limit:  cfg.Limit,
		Cursor: cfg.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return &ShowConcurrencyPoolResult{
		ConcurrencyPool: *pool,
		Queue:           queue.Leases,
		Pagination:      queue.Pagination,
	}, nil
}
