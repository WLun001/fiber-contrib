package pasetoware

import (
	"github.com/gofiber/fiber/v2"
	"strings"
)

// New PASETO middleware, returns a handler that takes a token in selected lookup param and in case token is valid
// it saves the decrypted token on ctx.Locals, take a look on Config to know more configuration options
func New(authConfigs ...Config) fiber.Handler {
	// Set default authConfig
	config := configDefault(authConfigs...)

	var extractor acquireToken
	switch config.TokenLookup[0] {
	case LookupHeader:
		extractor = acquireFromHeader
	case LookupQuery:
		extractor = acquireFromQuery
	case LookupParam:
		extractor = acquireFromParams
	case LookupCookie:
		extractor = acquireFromCookie
	default:
		extractor = acquireFromHeader
	}

	// Return middleware handler
	return func(c *fiber.Ctx) error {
		token := extractor(c, config.TokenLookup[1])
		// Filter request to skip middleware
		if config.Next != nil && config.Next(c) {
			return c.Next()
		}
		if len(token) <= 0 {
			return config.ErrorHandler(c, ErrMissingToken)
		}

		if len(config.TokenPrefix) > 0 {
			if strings.HasPrefix(token, config.TokenPrefix) {
				token = strings.TrimPrefix(token, config.TokenPrefix+" ")
			} else {
				return config.ErrorHandler(c, ErrIncorrectTokenPrefix)
			}
		}

		var decryptedData []byte
		err := pasetoObject.Decrypt(token, config.SymmetricKey, &decryptedData, nil)
		if err == nil {
			var payload interface{}
			payload, err = config.Validate(decryptedData)

			if err == nil {
				// Store user information from token into context.
				c.Locals(config.ContextKey, payload)
				return config.SuccessHandler(c)
			}
		}
		return config.ErrorHandler(c, err)
	}
}
