package bidi

import "context"

// Cookie is a cookie as the storage module serialises it. The value is a string here;
// the client does not use binary cookie values.
type Cookie struct {
	Name     string     `json:"name"`
	Value    BytesValue `json:"value"`
	Domain   string     `json:"domain"`
	Path     string     `json:"path"`
	HTTPOnly bool       `json:"httpOnly"`
	Secure   bool       `json:"secure"`
	// SameSite is strict, lax, none or default.
	SameSite string `json:"sameSite"`
	// Expiry is seconds since the epoch. Nil is a session cookie.
	Expiry *int64 `json:"expiry,omitempty"`
}

// GetCookies returns the cookies in the browser's default storage partition, which every tab
// the engine opens shares. A partition that names a context is not used: Firefox keeps those
// apart from the cookies a page sees.
func (c *Conn) GetCookies(ctx context.Context) ([]Cookie, error) {
	var out struct {
		Cookies []Cookie `json:"cookies"`
	}
	err := c.Call(ctx, "storage.getCookies", map[string]any{}, &out)
	return out.Cookies, err
}

// SetCookie stores one cookie in the default storage partition.
func (c *Conn) SetCookie(ctx context.Context, ck Cookie) error {
	cookie := map[string]any{
		"name":     ck.Name,
		"value":    ck.Value,
		"domain":   ck.Domain,
		"path":     ck.Path,
		"httpOnly": ck.HTTPOnly,
		"secure":   ck.Secure,
	}
	if ck.SameSite != "" {
		cookie["sameSite"] = ck.SameSite
	}
	if ck.Expiry != nil {
		cookie["expiry"] = *ck.Expiry
	}
	return c.Call(ctx, "storage.setCookie", map[string]any{"cookie": cookie}, nil)
}
