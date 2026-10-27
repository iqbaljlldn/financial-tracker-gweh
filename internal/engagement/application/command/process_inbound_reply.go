package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// ReviewConfirmer bridges engagement to financialcore to finalize pending review events.
type ReviewConfirmer interface {
	ConfirmReview(ctx context.Context, userID, eventID string) error
}

// InboundReplyCommand carries an incoming message from a WhatsApp webhook.
type InboundReplyCommand struct {
	SenderPhone string `json:"senderPhone"`
	MessageText string `json:"messageText"`
	ButtonID    string `json:"buttonId,omitempty"`
}

// InboundReplyResult holds the reply outcome.
type InboundReplyResult struct {
	MatchedIntent   string `json:"matchedIntent"` // "confirm", "edit", "clarify", "ignored"
	ResponseMessage string `json:"responseMessage"`
}

// ProcessInboundReplyHandler implements Constrained-Intent Reply Parsing (H4 Resolution) [GATE].
// Strictly no open NLU: uses structured button reply with strict whitelisted regex fallback.
type ProcessInboundReplyHandler struct {
	prefRepo        domain.NotificationPreferenceRepository
	jobRepo         domain.NotificationJobRepository
	messagingClient domain.MessagingClient
	confirmer       ReviewConfirmer
	logger          *slog.Logger
}

// NewProcessInboundReplyHandler constructs a new reply handler.
func NewProcessInboundReplyHandler(
	prefRepo domain.NotificationPreferenceRepository,
	jobRepo domain.NotificationJobRepository,
	messagingClient domain.MessagingClient,
	confirmer ReviewConfirmer,
	logger *slog.Logger,
) *ProcessInboundReplyHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProcessInboundReplyHandler{
		prefRepo:        prefRepo,
		jobRepo:         jobRepo,
		messagingClient: messagingClient,
		confirmer:       confirmer,
		logger:          logger,
	}
}

var (
	yesPattern = regexp.MustCompile(`^(?i)\s*(Y|YA|YES|CONFIRM|SETUJU|BENAR|OK)\s*$`)
	noPattern  = regexp.MustCompile(`^(?i)\s*(N|NO|TIDAK|T|EDIT|UBAH|BATAL)\s*$`)
)

// Handle processes the inbound reply and finalizes pending review event if confirmed.
func (h *ProcessInboundReplyHandler) Handle(ctx context.Context, cmd InboundReplyCommand) (*InboundReplyResult, error) {
	phone := strings.TrimSpace(cmd.SenderPhone)
	if phone == "" {
		return nil, fmt.Errorf("reply: sender phone is empty")
	}

	pref, err := h.prefRepo.FindByPhoneNumber(ctx, phone)
	if err != nil {
		return nil, fmt.Errorf("reply: finding user by phone %s: %w", phone, err)
	}
	if pref == nil {
		// Also try with/without leading 0 or 62
		var altPhone string
		if strings.HasPrefix(phone, "62") {
			altPhone = "0" + strings.TrimPrefix(phone, "62")
		} else if strings.HasPrefix(phone, "0") {
			altPhone = "62" + strings.TrimPrefix(phone, "0")
		}
		if altPhone != "" {
			pref, _ = h.prefRepo.FindByPhoneNumber(ctx, altPhone)
		}
	}

	if pref == nil {
		h.logger.WarnContext(ctx, "reply: received message from unknown phone number", "phone", phone)
		return &InboundReplyResult{
			MatchedIntent: "ignored",
		}, nil
	}

	btnID := strings.ToLower(strings.TrimSpace(cmd.ButtonID))
	text := strings.TrimSpace(cmd.MessageText)

	// 1. Structured button reply check [H4 primary]
	if btnID != "" {
		if strings.HasPrefix(btnID, "confirm") {
			eventID := extractEventID(btnID)
			return h.executeConfirmation(ctx, pref.UserID, phone, eventID)
		}
		if strings.HasPrefix(btnID, "edit") {
			return h.sendEditGuidance(ctx, phone)
		}
	}

	// 2. Strict whitelisted text pattern matching [H4 fallback]
	if yesPattern.MatchString(text) {
		return h.executeConfirmation(ctx, pref.UserID, phone, "")
	}

	if noPattern.MatchString(text) {
		return h.sendEditGuidance(ctx, phone)
	}

	// 3. Unrecognized reply -> send clarifying re-prompt, NEVER misinterpret or silent drop [GATE]
	clarifyMsg := "❓ Maaf, kami tidak mengenali perintah tersebut.\n\nKetik *Y* untuk konfirmasi transaksi terakhir, atau buka web Financial Tracker untuk melakukan koreksi."
	_ = h.sendQuickReply(ctx, phone, clarifyMsg)

	return &InboundReplyResult{
		MatchedIntent:   "clarify",
		ResponseMessage: clarifyMsg,
	}, nil
}

func (h *ProcessInboundReplyHandler) executeConfirmation(ctx context.Context, userID uuid.UUID, phone string, eventID string) (*InboundReplyResult, error) {
	// If eventID not in button, look up latest pending review job for user
	if eventID == "" {
		latestJob, err := h.jobRepo.FindLatestPendingReviewByUserID(ctx, userID)
		if err != nil || latestJob == nil {
			msg := "⚠️ Tidak ada transaksi tertunda (pending review) yang ditemukan untuk dikonfirmasi."
			_ = h.sendQuickReply(ctx, phone, msg)
			return &InboundReplyResult{MatchedIntent: "confirm", ResponseMessage: msg}, nil
		}

		var payload struct {
			Parameters map[string]string `json:"parameters"`
			Buttons    []struct {
				ID string `json:"id"`
			} `json:"buttons"`
		}
		if err := json.Unmarshal(latestJob.ComposedPayload(), &payload); err == nil {
			for _, b := range payload.Buttons {
				if strings.HasPrefix(b.ID, "confirm:") {
					eventID = strings.TrimPrefix(b.ID, "confirm:")
					break
				}
			}
		}
	}

	if eventID == "" {
		msg := "⚠️ Tidak dapat mengidentifikasi ID transaksi. Silakan buka aplikasi web untuk konfirmasi."
		_ = h.sendQuickReply(ctx, phone, msg)
		return &InboundReplyResult{MatchedIntent: "confirm", ResponseMessage: msg}, nil
	}

	// Finalize event in ledger via ReviewConfirmer
	if h.confirmer != nil {
		if err := h.confirmer.ConfirmReview(ctx, userID.String(), eventID); err != nil {
			h.logger.ErrorContext(ctx, "failed to finalize pending review event",
				"userId", userID,
				"eventId", eventID,
				"error", err,
			)
			errMsg := "⚠️ Terjadi kendala saat menyimpan konfirmasi ke pembukuan. Silakan coba lagi lewat aplikasi web."
			_ = h.sendQuickReply(ctx, phone, errMsg)
			return &InboundReplyResult{MatchedIntent: "confirm", ResponseMessage: errMsg}, nil
		}
	}

	successMsg := "✅ *Transaksi Berhasil Dikonfirmasi!*\nData telah disimpan dan dicatat secara permanen ke buku kas Anda."
	_ = h.sendQuickReply(ctx, phone, successMsg)

	h.logger.InfoContext(ctx, "confirmed financial event via WhatsApp reply",
		"userId", userID,
		"eventId", eventID,
	)

	return &InboundReplyResult{
		MatchedIntent:   "confirm",
		ResponseMessage: successMsg,
	}, nil
}

func (h *ProcessInboundReplyHandler) sendEditGuidance(ctx context.Context, phone string) (*InboundReplyResult, error) {
	msg := "✏️ *Koreksi Transaksi*\nUntuk mengoreksi nominal, kategori, atau merchant, silakan buka menu Transaksi di aplikasi Financial Tracker."
	_ = h.sendQuickReply(ctx, phone, msg)
	return &InboundReplyResult{
		MatchedIntent:   "edit",
		ResponseMessage: msg,
	}, nil
}

func (h *ProcessInboundReplyHandler) sendQuickReply(ctx context.Context, phone, text string) error {
	if h.messagingClient == nil {
		return nil
	}
	_, err := h.messagingClient.SendMessage(ctx, domain.OutboundMessage{
		RecipientPhone:   phone,
		NotificationType: domain.NotificationTypeReviewRequest,
		RenderedText:     text,
	})
	return err
}

func extractEventID(buttonID string) string {
	parts := strings.Split(buttonID, ":")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
