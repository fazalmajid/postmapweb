package main

import (
	"encoding/base64"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/labstack/echo"
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
		h.Set(echo.HeaderContentSecurityPolicy,
			"default-src 'self'; img-src 'self' data: 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'",
		)
		h.Set("X-Frame-Options", "\"SAMEORIGIN\" always")
		h.Set("X-Xss-Protection", "\"1; mode=block\" always")
		h.Set("X-Content-Type-Options", "\"nosniff\" always")
		h.Set("Referrer-Policy", "no-referrer-when-downgrade")
		h.Set("Feature-Policy",
			"accelerometer 'none'; ambient-light-sensor 'none'; autoplay 'none'; battery 'none'; camera 'none'; display-capture 'none'; document-domain 'none'; encrypted-media 'none'; execution-while-not-rendered 'none'; execution-while-out-of-viewport 'none'; fullscreen 'none'; geolocation 'none'; gyroscope 'none'; layout-animations 'none'; legacy-image-formats 'none'; magnetometer 'none'; microphone 'none'; midi 'none'; navigation-override 'none'; oversized-images 'none'; payment 'none'; picture-in-picture 'none'; publickey-credentials-get 'none'; sync-xhr 'none'; usb 'none'; vr 'none'; wake-lock 'none'; screen-wake-lock 'none'; web-share 'none'; xr-spatial-tracking 'none'; notifications 'none'; push 'none'; speaker 'none'; vibrate 'none'; payment 'none'")
		h.Set("Permissions-Policy",
			"accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), cross-origin-isolated=(), display-capture=(), document-domain=(), encrypted-media=(), execution-while-not-rendered=(), execution-while-out-of-viewport=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), navigation-override=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=(), clipboard-read=(), clipboard-write=(), gamepad=(), speaker-selection=(), conversion-measurement=(), focus-without-user-activation=(), hid=(), idle-detection=(), serial=(), sync-script=(), trust-token-redemption=(), vertical-scroll=(), notifications=(), push=(), speaker=(), vibrate=(), interest-cohort=()")

		err := next(c)
		if err != nil {
			c.Error(err)
		}

		return nil
	}
}
