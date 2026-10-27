package classifier_test

import (
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/classifier"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestEvaluateTransactionalContent(t *testing.T) {
	tests := []struct {
		name          string
		raw           domain.RawEmailMessage
		wantDiscarded bool
	}{
		{
			name: "Fonnte registration notification without transaction",
			raw: domain.RawEmailMessage{
				Sender:   "No Reply Fonnte <noreply@fonnte.com>",
				Subject:  "Pendaftaran Akun Fonnte Berhasil",
				BodyText: "Halo, selamat datang di Fonnte. Akun Anda berhasil dibuat. Silakan login ke dashboard.",
			},
			wantDiscarded: true,
		},
		{
			name: "IDCloudHost support ticket update",
			raw: domain.RawEmailMessage{
				Sender:   "IDCloudHost <care@idcloudhost.com>",
				Subject:  "[Ticket #12345] Bantuan Teknis Cloud Hosting",
				BodyText: "Halo, tim support kami telah menjawab tiket Anda. Silakan periksa.",
			},
			wantDiscarded: true,
		},
		{
			name: "OTP verification email",
			raw: domain.RawEmailMessage{
				Sender:   "security@bank.com",
				Subject:  "Kode OTP Masuk Akun Anda: 593821",
				BodyText: "Jangan berikan kode ini kepada siapapun.",
			},
			wantDiscarded: true,
		},
		{
			name: "Marketing newsletter with List-Unsubscribe header",
			raw: domain.RawEmailMessage{
				Sender:   "promo@store.com",
				Subject:  "Promo Diskon Akhir Pekan",
				BodyText: "Dapatkan penawaran menarik hanya minggu ini.",
				Headers: map[string]string{
					"List-Unsubscribe": "<mailto:unsub@store.com>",
				},
			},
			wantDiscarded: true,
		},
		{
			name: "Genuine Amazon purchase confirmation",
			raw: domain.RawEmailMessage{
				Sender:   "orders@amazon.com",
				Subject:  "Your Purchase Confirmation",
				BodyText: "Thank you. Your card was charged $45.00 on Aug 3, 2026.",
			},
			wantDiscarded: false,
		},
		{
			name: "Genuine GoPay transfer receipt",
			raw: domain.RawEmailMessage{
				Sender:   "receipts@gojek.com",
				Subject:  "Bukti Transaksi GoPay",
				BodyText: "Pembayaran berhasil sebesar Rp 25.000 ke Kopi Kenangan.",
			},
			wantDiscarded: false,
		},
		{
			name: "IDCloudHost actual paid invoice receipt",
			raw: domain.RawEmailMessage{
				Sender:   "billing@idcloudhost.com",
				Subject:  "Bukti Pembayaran Invoice #998811",
				BodyText: "Pembayaran berhasil sebesar Rp 150.000 untuk perpanjangan domain.",
			},
			wantDiscarded: false,
		},
		{
			name: "Ambiguous email with amount - should route to AI",
			raw: domain.RawEmailMessage{
				Sender:   "custom-store@mystore.com",
				Subject:  "Informasi Pesanan Anda",
				BodyText: "Total pesanan Anda adalah Rp 75.000. Terima kasih atas pesanannya.",
			},
			wantDiscarded: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isDiscarded, reason := classifier.EvaluateTransactionalContent(tc.raw)
			if isDiscarded != tc.wantDiscarded {
				t.Errorf("EvaluateTransactionalContent() discarded = %v, want %v (reason: %s)", isDiscarded, tc.wantDiscarded, reason)
			}
		})
	}
}
