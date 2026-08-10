package qris

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	qrislib "github.com/akbarhabiby/go-qris"
	"github.com/skip2/go-qrcode"
	"xloyal/backend/internal/domain"
)

var (
	ErrStaticRequired = errors.New("uploaded QRIS must be static")
	ErrInvalidQRIS    = errors.New("invalid QRIS payload")
)

func DecodeImage(data []byte, mime string) (domain.QRISTemplate, error) {
	if len(data) == 0 || len(data) > 5<<20 {
		return domain.QRISTemplate{}, errors.New("image must be between 1 byte and 5 MB")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return domain.QRISTemplate{}, fmt.Errorf("invalid image: %w", err)
	}
	ext := ".png"
	if strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg") {
		ext = ".jpg"
	}
	file, err := os.CreateTemp("", "xloyal-qris-*"+ext)
	if err != nil {
		return domain.QRISTemplate{}, err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return domain.QRISTemplate{}, err
	}
	if err = file.Close(); err != nil {
		return domain.QRISTemplate{}, err
	}
	parsed, err := qrislib.NewQRISFromImage(path)
	if err != nil {
		return domain.QRISTemplate{}, fmt.Errorf("decode QRIS image: %w", err)
	}
	payload := parsed.Serialize()
	if !parsed.IsStatic() || parsed.Get(qrislib.TagTransactionCurrency) != "360" || !validCRC(payload) {
		return domain.QRISTemplate{}, ErrStaticRequired
	}
	info := parsed.MapToStruct()
	return domain.QRISTemplate{
		StaticPayload: payload,
		ImageMIME:     mime,
		MerchantName:  info.MerchantName,
		MerchantCity:  info.MerchantCity,
	}, nil
}

func Convert(staticPayload string, amount int64) (string, error) {
	if amount <= 0 || amount > 100_000_000 {
		return "", errors.New("amount must be between 1 and 100000000")
	}
	parsed, err := qrislib.NewQRISFromString(staticPayload)
	if err != nil {
		return "", fmt.Errorf("parse static QRIS: %w", err)
	}
	if !parsed.IsStatic() || !validCRC(parsed.Serialize()) {
		return "", ErrStaticRequired
	}
	parsed.SetAmountWithOptions(qrislib.QRISAmountOptions{Amount: int(amount)})
	dynamic := parsed.Serialize()
	if !strings.HasPrefix(dynamic, "000201010212") || !validCRC(dynamic) {
		return "", ErrInvalidQRIS
	}
	return dynamic, nil
}

func PNG(payload string) ([]byte, error) {
	return qrcode.Encode(payload, qrcode.Medium, 320)
}

func validCRC(payload string) bool {
	if len(payload) < 8 || !strings.HasSuffix(payload[:len(payload)-4], "6304") {
		return false
	}
	want := qrislib.CalculateCRC(payload[:len(payload)-4])
	return strings.EqualFold(want, payload[len(payload)-4:])
}
