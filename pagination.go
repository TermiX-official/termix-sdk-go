package termix

import "context"

// Page is the backend's list wrapper.
type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// Paginate walks every page and returns all items.
func Paginate[T any](ctx context.Context, fetch func(context.Context, int) (Page[T], error)) ([]T, error) {
	var all []T
	page := 1
	for {
		p, err := fetch(ctx, page)
		if err != nil {
			return all, err
		}
		all = append(all, p.Items...)
		if len(p.Items) == 0 || page >= p.TotalPages {
			return all, nil
		}
		page++
	}
}
