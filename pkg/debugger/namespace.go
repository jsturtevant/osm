package debugger

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type namespaces struct {
	Namespaces []string `json:"namespaces"`
}

func (ds DebugConfig) getMonitoredNamespacesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n namespaces
		var err error
		n.Namespaces, err = ds.meshCatalog.ListNamespaces()
		if err != nil {
			log.Error().Err(err).Msgf("Error marshalling policy %+v", n)
		}

		jsonPolicies, err := json.Marshal(n)
		if err != nil {
			log.Error().Err(err).Msgf("Error marshalling policy %+v", n)
		}

		_, _ = fmt.Fprint(w, string(jsonPolicies))
	})
}
