package loans

import (
	"errors"
	"net/http"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/httpx"
)

func RegisterReturnRoute(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /loans/{id}/return", h.returnLoan)
}

func (h *Handler) returnLoan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathInt[int64](r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	loan, err := h.svc.Return(r.Context(), id)
	if errors.Is(err, ErrAlreadyReturned) {
		err = httpx.NewError(http.StatusConflict, "the loan was already returned")
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, loan)
}
