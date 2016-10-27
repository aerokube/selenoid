package ensure

import "net/http"

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

func Post(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	}
}
