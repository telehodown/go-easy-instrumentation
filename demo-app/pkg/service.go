package pkg

import (
	"net/http"
)

func Service() error {
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		return err
	}

	// Make an http request to an external address
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}
