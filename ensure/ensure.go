package ensure

import "net/http"

// CloseNotifier - handler to ensure http.CloseNotifier implementation
func CloseNotifier(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := w.(http.CloseNotifier)
		if !ok {
			http.Error(w, "unable to handle client close notifications", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// Post - handler to allow only POST methods
func Post(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	}
}
