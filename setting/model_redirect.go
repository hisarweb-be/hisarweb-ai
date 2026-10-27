package setting

import (
	"encoding/json"
	"errors"
	"maps"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// ModelRedirects maps a retired model name to its successor in the current
// assortment. The distributor applies it before channel selection, so a
// request for an old generation is selected, billed and served as the
// successor: the customer always gets the newest model of the same family at
// the newest price.
var modelRedirects = map[string]string{}
var modelRedirectsMutex sync.RWMutex

func GetModelRedirect(model string) (string, bool) {
	modelRedirectsMutex.RLock()
	defer modelRedirectsMutex.RUnlock()
	target, ok := modelRedirects[model]
	return target, ok
}

func ModelRedirectsCopy() map[string]string {
	modelRedirectsMutex.RLock()
	defer modelRedirectsMutex.RUnlock()
	redirectsCopy := make(map[string]string, len(modelRedirects))
	maps.Copy(redirectsCopy, modelRedirects)
	return redirectsCopy
}

func ModelRedirects2JSONString() string {
	modelRedirectsMutex.RLock()
	defer modelRedirectsMutex.RUnlock()
	jsonBytes, err := json.Marshal(modelRedirects)
	if err != nil {
		common.SysLog(common.LogText("error marshaling model redirects: %s", err.Error()))
		return "{}"
	}
	return string(jsonBytes)
}

// UpdateModelRedirectsByJSONString replaces the redirect table. Keys and
// targets must be non-empty, a model must not map to itself, and chains must
// never loop: any of those would resolve a request to an unservable model.
func UpdateModelRedirectsByJSONString(jsonStr string) error {
	redirects := make(map[string]string)
	if err := json.Unmarshal([]byte(jsonStr), &redirects); err != nil {
		return err
	}
	for from, to := range redirects {
		if from == "" || to == "" {
			return errors.New("model redirect keys and targets must not be empty")
		}
		if from == to {
			return errors.New("model redirect must not map a model to itself: " + from)
		}
	}
	for from := range redirects {
		visited := map[string]bool{from: true}
		current := from
		for {
			next, ok := redirects[current]
			if !ok {
				break
			}
			if visited[next] {
				return errors.New("model redirect contains a cycle at " + from)
			}
			visited[next] = true
			current = next
		}
	}
	modelRedirectsMutex.Lock()
	defer modelRedirectsMutex.Unlock()
	modelRedirects = redirects
	return nil
}
