package controller

import (
	"net/http"
	"net/url"

	"github.com/imyousuf/appcommons/data"
)

func GetPagination(req *http.Request) *data.Pagination {
	result := &data.Pagination{}
	originalURL := req.URL
	previous := originalURL.Query().Get(PreviousPaginationQueryParamKey)
	if len(previous) > 0 {
		prevCursor, err := data.ParseCursor(previous)
		if err == nil {
			result.Previous = prevCursor
		}
	}
	next := originalURL.Query().Get(NextPaginationQueryParamKey)
	if len(next) > 0 {
		nextCursor, err := data.ParseCursor(next)
		if err == nil {
			result.Next = nextCursor
		}
	}
	return result
}

func GetPaginationLinks(req *http.Request, pagination *data.Pagination) map[string]string {
	links := make(map[string]string)
	if pagination != nil {
		originalURL := req.URL
		if pagination.Previous != nil {
			previous := cloneBaseURL(originalURL)
			prevQueries := make(url.Values)
			prevQueries.Set(PreviousPaginationQueryParamKey, pagination.Previous.String())
			previous.RawQuery = prevQueries.Encode()
			links[PreviousPaginationQueryParamKey] = previous.String()
		}
		if pagination.Next != nil {
			next := cloneBaseURL(originalURL)
			nextQueries := make(url.Values)
			nextQueries.Set(NextPaginationQueryParamKey, pagination.Next.String())
			next.RawQuery = nextQueries.Encode()
			links[NextPaginationQueryParamKey] = next.String()
		}
	}
	return links
}

func cloneBaseURL(originalURL *url.URL) *url.URL {
	newURL := &url.URL{}
	newURL.Scheme = originalURL.Scheme
	newURL.Host = originalURL.Host
	newURL.Path = originalURL.Path
	newURL.RawPath = originalURL.RawPath
	return newURL
}
