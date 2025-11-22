package main

import (
	"encoding/base64"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/labstack/echo/v4"
)

// BasicAuth returns an HTTP basic authentication middleware.
//
// For valid credentials it calls the next handler.
// For invalid credentials, it sends "401 - Unauthorized" response.
const (
	Basic = "Basic"
)

func BasicAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		auth := c.Request().Header.Get(echo.HeaderAuthorization)
		l := len(Basic)

		if len(auth) > l+1 && auth[:l] == Basic {
			b, err := base64.StdEncoding.DecodeString(auth[l+1:])
			if err == nil {
				cred := string(b)
				for i := 0; i < len(cred); i++ {
					if cred[i] == ':' {
						// Verify credentials
						for _, d := range conf.Domains {
							if cred[:i] == d.Name && bcrypt.CompareHashAndPassword([]byte(d.PassHash), []byte(cred[i+1:])) == nil {
								c.Set("domain", d)
								err := next(c)
								if err != nil {
									c.Error(err)
								}
								return err
							}
						}
					}
				}
			}
		}
		c.Response().Header().Set(echo.HeaderWWWAuthenticate, Basic+" realm=Restricted")
		return echo.NewHTTPError(http.StatusUnauthorized)
	}
}

func SecurityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		h := c.Response().Header()
		// every security header apart from HSTS
		// Allow inline styles for Simple DataTables
		h.Set(echo.HeaderContentSecurityPolicy,
			"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'",
		)
		h.Set(echo.HeaderXFrameOptions, "\"SAMEORIGIN\" always")
		h.Set(echo.HeaderXXSSProtection, "\"1; mode=block\" always")
		h.Set(echo.HeaderXContentTypeOptions, "\"nosniff\" always")
		h.Set(echo.HeaderReferrerPolicy, "no-referrer-when-downgrade")
		// Only use Permissions-Policy (Feature-Policy is deprecated)
		// Only include features recognized by modern browsers
		h.Set("Permissions-Policy",
			"accelerometer=(), autoplay=(), camera=(), cross-origin-isolated=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), xr-spatial-tracking=(), clipboard-read=(), clipboard-write=(), gamepad=(), hid=(), idle-detection=(), serial=(), interest-cohort=()")

		err := next(c)
		if err != nil {
			c.Error(err)
		}

		return nil
	}
}
