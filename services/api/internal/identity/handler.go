package identity

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler serves the endpoints document 28 lists.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Routes registers the identity surface under the versioned prefix.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix

	// Unauthenticated: these are how a caller becomes authenticated.
	mux.HandleFunc("POST "+p+"/auth/otp/request", h.requestOTP)
	mux.HandleFunc("POST "+p+"/auth/otp/verify", h.verifyOTP)
	mux.HandleFunc("POST "+p+"/auth/refresh", h.refresh)

	// Authenticated.
	mux.Handle("POST "+p+"/auth/logout", authenticate(http.HandlerFunc(h.logout)))
	mux.Handle("POST "+p+"/auth/logout-all", authenticate(http.HandlerFunc(h.logoutAll)))
	mux.Handle("GET "+p+"/me", authenticate(http.HandlerFunc(h.me)))
	mux.Handle("PATCH "+p+"/me", authenticate(http.HandlerFunc(h.updateMe)))
	mux.Handle("GET "+p+"/me/sessions", authenticate(http.HandlerFunc(h.sessions)))

	// Administrative. Role authorization is applied here; SUPPORT is
	// deliberately excluded — document 20 says support permissions are
	// narrower than admin, and granting roles is the boundary where that
	// matters most.
	mux.Handle("POST "+p+"/admin/users/{id}/roles",
		authenticate(RequireRole(RoleAdmin, RoleSuperAdmin)(http.HandlerFunc(h.grantRole))))
	mux.Handle("DELETE "+p+"/admin/users/{id}/roles/{role}",
		authenticate(RequireRole(RoleAdmin, RoleSuperAdmin)(http.HandlerFunc(h.revokeRole))))
}

// --- request and response bodies ---------------------------------------------

type otpRequestBody struct {
	Phone   string `json:"phone"`
	Purpose string `json:"purpose,omitempty"`
}

type otpRequestResponse struct {
	// Deliberately says nothing about whether the account exists (document 28:
	// prevent account enumeration).
	ExpiresAt time.Time `json:"expires_at"`
}

type otpVerifyBody struct {
	Phone   string `json:"phone"`
	Code    string `json:"code"`
	Purpose string `json:"purpose,omitempty"`
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresAt    time.Time    `json:"expires_at"`
	User         userResponse `json:"user"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u User) userResponse {
	roles := u.RoleStrings()
	if roles == nil {
		roles = []string{}
	}
	return userResponse{
		ID: u.ID, Phone: u.Phone, Email: u.Email, Name: u.Name,
		AvatarURL: u.AvatarURL, Status: string(u.Status), Roles: roles,
		CreatedAt: u.CreatedAt,
	}
}

type sessionResponse struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

type updateProfileBody struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type roleBody struct {
	Role string `json:"role"`
}

// --- handlers ----------------------------------------------------------------

func (h *Handler) requestOTP(w http.ResponseWriter, r *http.Request) {
	var body otpRequestBody
	if !decode(w, r, &body) {
		return
	}
	purpose := OTPPurpose(body.Purpose)
	if purpose == "" {
		purpose = PurposeLogin
	}

	expiresAt, err := h.service.RequestOTP(r.Context(), body.Phone, purpose, RequestContextFrom(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusAccepted, otpRequestResponse{ExpiresAt: expiresAt})
}

func (h *Handler) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var body otpVerifyBody
	if !decode(w, r, &body) {
		return
	}
	purpose := OTPPurpose(body.Purpose)
	if purpose == "" {
		purpose = PurposeLogin
	}

	tokens, err := h.service.VerifyOTP(r.Context(), body.Phone, body.Code, purpose, RequestContextFrom(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	writeTokens(w, r, http.StatusOK, tokens)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body refreshBody
	if !decode(w, r, &body) {
		return
	}
	if body.RefreshToken == "" {
		httpx.WriteError(w, r, httpx.Validation("a refresh token is required",
			map[string]string{"refresh_token": "required"}))
		return
	}
	tokens, err := h.service.Refresh(r.Context(), body.RefreshToken, RequestContextFrom(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	writeTokens(w, r, http.StatusOK, tokens)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	if err := h.service.Logout(r.Context(), principal.SessionID, principal.UserID, RequestContextFrom(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	count, err := h.service.LogoutAll(r.Context(), principal.UserID, RequestContextFrom(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]int64{"sessions_revoked": count})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	user, err := h.service.Me(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, toUserResponse(user))
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	var body updateProfileBody
	if !decode(w, r, &body) {
		return
	}
	// Phone is absent from the body by design: changing it is an identity
	// change, and document 20 requires stronger verification for one. That
	// flow uses PurposePhoneChange and does not exist yet.
	user, err := h.service.UpdateProfile(r.Context(), principal.UserID, body.Name, body.Email, body.AvatarURL)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, toUserResponse(user))
}

func (h *Handler) sessions(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	sessions, err := h.service.Sessions(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionResponse{
			ID: s.ID, DeviceID: s.DeviceID, CreatedAt: s.CreatedAt,
			LastUsedAt: s.LastUsedAt, ExpiresAt: s.ExpiresAt,
			Current: s.ID == principal.SessionID,
		})
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) grantRole(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	var body roleBody
	if !decode(w, r, &body) {
		return
	}
	role := Role(body.Role)
	// Only a super admin may create another admin. An ADMIN who could grant
	// ADMIN or SUPER_ADMIN could promote themselves past every check above
	// them, which makes the distinction between the two roles meaningless.
	if (role == RoleAdmin || role == RoleSuperAdmin) && !principal.HasRole(RoleSuperAdmin) {
		httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
		return
	}
	if err := h.service.GrantRole(r.Context(), principal.UserID, r.PathValue("id"), role, RequestContextFrom(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) revokeRole(w http.ResponseWriter, r *http.Request) {
	principal := MustPrincipal(r.Context())
	role := Role(r.PathValue("role"))
	if (role == RoleAdmin || role == RoleSuperAdmin) && !principal.HasRole(RoleSuperAdmin) {
		httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
		return
	}
	if err := h.service.RevokeRole(r.Context(), principal.UserID, r.PathValue("id"), role, RequestContextFrom(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusNoContent, nil)
}

// --- helpers -----------------------------------------------------------------

// decode reads a JSON body, rejecting unknown fields so a client typo is
// reported rather than silently ignored.
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.WriteError(w, r, httpx.Validation("the request body is not valid",
			map[string]string{"body": err.Error()}))
		return false
	}
	return true
}

func writeTokens(w http.ResponseWriter, r *http.Request, status int, tokens Tokens) {
	httpx.WriteJSON(w, r, status, tokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    tokens.ExpiresAt,
		User:         toUserResponse(tokens.User),
	})
}
