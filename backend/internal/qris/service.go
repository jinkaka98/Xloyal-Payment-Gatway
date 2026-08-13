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

// billNumberTag is the Tag 62 Additional Data sub-tag used for the bill
// number (EMVCo "Bill Number" data object ID "01").
const billNumberTag = "01"

// BillNumber extracts the bill number from a QRIS payload's Tag 62
// Additional Data template. It returns an empty string when the payload has
// no bill number.
func BillNumber(payload string) (string, error) {
	parsed, err := qrislib.NewQRISFromString(payload)
	if err != nil {
		return "", fmt.Errorf("parse QRIS: %w", err)
	}
	return parsed.MapToStruct().AdditionalData[billNumberTag], nil
}

// WithBillNumber injects a bill number into the Tag 62 Additional Data
// template of a QRIS payload and returns a re-serialized payload with a
// freshly calculated CRC. An existing bill number is replaced.
func WithBillNumber(payload, billNumber string) (string, error) {
	if billNumber == "" {
		return "", errors.New("bill number must not be empty")
	}
	tlvs, err := qrislib.ParseTLV(payload)
	if err != nil {
		return "", fmt.Errorf("parse QRIS: %w", err)
	}

	var additional *qrislib.TLV
	for i := range tlvs {
		if tlvs[i].Tag == string(qrislib.TagAdditionalData) {
			additional = &tlvs[i]
			break
		}
	}

	var subs []qrislib.TLV
	if additional != nil {
		if subs, err = qrislib.ParseTLV(additional.Value); err != nil {
			return "", fmt.Errorf("parse additional data: %w", err)
		}
	}

	replaced := false
	for i := range subs {
		if subs[i].Tag == billNumberTag {
			subs[i].Value = billNumber
			subs[i].Len = len(billNumber)
			replaced = true
			break
		}
	}
	if !replaced {
		subs = append(subs, qrislib.TLV{Tag: billNumberTag, Len: len(billNumber), Value: billNumber})
	}

	value := qrislib.SerializeTLV(subs)
	if additional != nil {
		additional.Value = value
		additional.Len = len(value)
	} else {
		added := qrislib.TLV{Tag: string(qrislib.TagAdditionalData), Len: len(value), Value: value}
		out := make([]qrislib.TLV, 0, len(tlvs)+1)
		inserted := false
		for _, tlv := range tlvs {
			if tlv.Tag == string(qrislib.TagCRC) {
				out = append(out, added)
				inserted = true
			}
			out = append(out, tlv)
		}
		if !inserted {
			out = append(out, added)
		}
		tlvs = out
	}

	tlvs = qrislib.UpdateCRC(tlvs)
	return qrislib.SerializeTLV(tlvs), nil
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
