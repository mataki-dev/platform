package search

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mataki-dev/platform/errors"
)

// OperationOption configures a search operation registration.
type OperationOption func(*operationConfig)

type operationConfig struct {
	tag         string
	summary     string
	description string
	middleware  []func(ctx huma.Context, next func(huma.Context))
}

// WithTag sets the operation tag.
func WithTag(tag string) OperationOption {
	return func(c *operationConfig) { c.tag = tag }
}

// WithSummary sets the operation summary.
func WithSummary(summary string) OperationOption {
	return func(c *operationConfig) { c.summary = summary }
}

// WithDescription sets the operation description.
func WithDescription(desc string) OperationOption {
	return func(c *operationConfig) { c.description = desc }
}

// WithMiddleware adds per-operation middleware.
func WithMiddleware(mw ...func(ctx huma.Context, next func(huma.Context))) OperationOption {
	return func(c *operationConfig) { c.middleware = append(c.middleware, mw...) }
}

// SearchInput wraps SearchRequest as a Huma request body.
type SearchInput struct {
	Body SearchRequest
}

// SearchHandler is the function signature products implement.
type SearchHandler[T any] func(ctx context.Context, req ValidatedSearch) (*SearchResponse[T], error)

// RegisterSearchOperation registers a POST /{resource}/search endpoint
// with standard validation, error handling, and OpenAPI metadata.
func RegisterSearchOperation[T any](
	api huma.API,
	path string,
	schema ResourceSchema,
	handler SearchHandler[T],
	opts ...OperationOption,
) {
	cfg := &operationConfig{
		summary: fmt.Sprintf("Search %s", path),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	op := huma.Operation{
		Method:      http.MethodPost,
		Path:        path,
		Summary:     cfg.summary,
		Description: cfg.description,
		Middlewares: huma.Middlewares(cfg.middleware),
	}
	if cfg.tag != "" {
		op.Tags = []string{cfg.tag}
	}

	huma.Register(api, op, func(ctx context.Context, input *SearchInput) (*struct{ Body *SearchResponse[T] }, error) {
		validated, validationErrs := Validate(input.Body, schema)
		if len(validationErrs) > 0 {
			fieldErrs := make([]errors.FieldError, len(validationErrs))
			for i, ve := range validationErrs {
				fieldErrs[i] = errors.FieldError{
					Field:   ve.Field,
					Code:    ve.Code,
					Message: ve.Message,
				}
			}
			return nil, errors.ToHumaError(
				errors.NewInvalidInput(
					"The search request contains invalid parameters.",
					errors.WithFieldErrors(fieldErrs...),
				),
			)
		}

		resp, err := handler(ctx, validated)
		if err != nil {
			if e, ok := err.(*errors.SemanticError); ok {
				return nil, errors.ToHumaError(e)
			}
			return nil, errors.ToHumaError(
				errors.NewInternal("An unexpected error occurred.", errors.WithCause(err)),
			)
		}

		return &struct{ Body *SearchResponse[T] }{Body: resp}, nil
	})
}
