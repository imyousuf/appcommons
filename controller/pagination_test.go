package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/imyousuf/appcommons/data"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestGetPaginationLinks(t *testing.T) {
	t.Run("Both", func(t *testing.T) {
		t.Parallel()
		paginateable := data.BasePaginateable{}
		paginateable.CreatedAt = time.Now()
		paginateable.UpdatedAt = time.Now()
		paginateable.ID = xid.New()
		pagination := data.NewPagination(&paginateable, &paginateable)
		getReq, _ := http.NewRequest("GET", "/client", nil)
		links := GetPaginationLinks(getReq, pagination)
		assert.NotNil(t, links[PreviousPaginationQueryParamKey])
		assert.Contains(t, links[PreviousPaginationQueryParamKey], PreviousPaginationQueryParamKey)
		assert.NotNil(t, links[NextPaginationQueryParamKey])
		assert.Contains(t, links[NextPaginationQueryParamKey], NextPaginationQueryParamKey)
	})
	t.Run("Previous", func(t *testing.T) {
		t.Parallel()
		paginateable := data.BasePaginateable{}
		paginateable.CreatedAt = time.Now()
		paginateable.UpdatedAt = time.Now()
		paginateable.ID = xid.New()
		pagination := data.NewPagination(nil, &paginateable)
		getReq, _ := http.NewRequest("GET", "/client", nil)
		links := GetPaginationLinks(getReq, pagination)
		assert.NotNil(t, links[PreviousPaginationQueryParamKey])
		assert.Contains(t, links[PreviousPaginationQueryParamKey], PreviousPaginationQueryParamKey)
		_, ok := links[NextPaginationQueryParamKey]
		assert.False(t, ok)
	})
	t.Run("Next", func(t *testing.T) {
		t.Parallel()
		paginateable := data.BasePaginateable{}
		paginateable.CreatedAt = time.Now()
		paginateable.UpdatedAt = time.Now()
		paginateable.ID = xid.New()
		pagination := data.NewPagination(&paginateable, nil)
		getReq, _ := http.NewRequest("GET", "/client", nil)
		links := GetPaginationLinks(getReq, pagination)
		_, ok := links[PreviousPaginationQueryParamKey]
		assert.False(t, ok)
		assert.NotNil(t, links[NextPaginationQueryParamKey])
		assert.Contains(t, links[NextPaginationQueryParamKey], NextPaginationQueryParamKey)
	})
}

func TestGetPagination(t *testing.T) {
	t.Run("Previous", func(t *testing.T) {
		t.Parallel()
		paginateable := data.BasePaginateable{}
		paginateable.CreatedAt = time.Now()
		paginateable.UpdatedAt = time.Now()
		paginateable.ID = xid.New()
		pagination := data.NewPagination(nil, &paginateable)
		getReq, _ := http.NewRequest("GET", "/client", nil)
		links := GetPaginationLinks(getReq, pagination)
		pageReq, _ := http.NewRequest("GET", links[PreviousPaginationQueryParamKey], nil)
		pagination = GetPagination(pageReq)
		assert.NotNil(t, pagination.Previous)
		assert.Nil(t, pagination.Next)
	})
	t.Run("Next", func(t *testing.T) {
		t.Parallel()
		paginateable := data.BasePaginateable{}
		paginateable.CreatedAt = time.Now()
		paginateable.UpdatedAt = time.Now()
		paginateable.ID = xid.New()
		pagination := data.NewPagination(&paginateable, nil)
		getReq, _ := http.NewRequest("GET", "/client", nil)
		links := GetPaginationLinks(getReq, pagination)
		pageReq, _ := http.NewRequest("GET", links[NextPaginationQueryParamKey], nil)
		pagination = GetPagination(pageReq)
		assert.NotNil(t, pagination.Next)
		assert.Nil(t, pagination.Previous)
	})
}
