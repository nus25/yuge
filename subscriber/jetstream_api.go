package subscriber

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type JetstreamApiHandler struct {
	controller JetstreamController
}

func NewJetstreamApiHandler(controller JetstreamController) *JetstreamApiHandler {
	if controller == nil {
		controller = NewUnavailableJetstreamController()
	}
	return &JetstreamApiHandler{controller: controller}
}

func (h *JetstreamApiHandler) Connect(c *gin.Context) {
	var req JetstreamConnectRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			respondWithError(c, http.StatusBadRequest, "invalid request body", err)
			return
		}
	}

	status, err := h.controller.Connect(req)
	if err != nil {
		if errors.Is(err, ErrJetstreamControllerUnavailable) {
			respondWithError(c, http.StatusServiceUnavailable, "jetstream controller is not configured", nil)
			return
		}
		respondWithError(c, http.StatusInternalServerError, "failed to connect jetstream", err)
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *JetstreamApiHandler) Disconnect(c *gin.Context) {
	status, err := h.controller.Disconnect()
	if err != nil {
		if errors.Is(err, ErrJetstreamControllerUnavailable) {
			respondWithError(c, http.StatusServiceUnavailable, "jetstream controller is not configured", nil)
			return
		}
		respondWithError(c, http.StatusInternalServerError, "failed to disconnect jetstream", err)
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *JetstreamApiHandler) Status(c *gin.Context) {
	if IsUnavailableJetstreamController(h.controller) {
		respondWithError(c, http.StatusServiceUnavailable, "jetstream controller is not configured", nil)
		return
	}

	c.JSON(http.StatusOK, h.controller.Status())
}
