package qris

import (
	"strings"
	"testing"
)

const staticFixture = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"

func TestConvertStaticToDynamic(t *testing.T) {
	got, err := Convert(staticFixture, 50000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "000201010212") || !strings.Contains(got, "540550000") || !validCRC(got) {
		t.Fatalf("invalid dynamic payload: %s", got)
	}
}

func TestConvertRejectsInvalidAndNonStatic(t *testing.T) {
	if _, err := Convert(strings.Replace(staticFixture, "9095", "0000", 1), 50000); err == nil {
		t.Fatal("invalid CRC accepted")
	}
	dynamic, err := Convert(staticFixture, 50000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(dynamic, 50000); err == nil {
		t.Fatal("dynamic payload accepted as static template")
	}
}

func TestWithBillNumberAndBillNumber(t *testing.T) {
	dynamic, err := Convert(staticFixture, 50000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := WithBillNumber(dynamic, "INV12345")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "000201010212") || !validCRC(got) {
		t.Fatalf("invalid payload with bill number: %s", got)
	}
	bn, err := BillNumber(got)
	if err != nil {
		t.Fatal(err)
	}
	if bn != "INV12345" {
		t.Fatalf("bill number = %q, want INV12345", bn)
	}

	replaced, err := WithBillNumber(got, "INV99999")
	if err != nil {
		t.Fatal(err)
	}
	if bn, err = BillNumber(replaced); err != nil || bn != "INV99999" {
		t.Fatalf("replaced bill number = %q (err=%v), want INV99999", bn, err)
	}
	if !validCRC(replaced) {
		t.Fatalf("invalid CRC after replacing bill number: %s", replaced)
	}
}

func TestBillNumberMissing(t *testing.T) {
	dynamic, err := Convert(staticFixture, 50000)
	if err != nil {
		t.Fatal(err)
	}
	bn, err := BillNumber(dynamic)
	if err != nil {
		t.Fatal(err)
	}
	if bn != "" {
		t.Fatalf("bill number = %q, want empty", bn)
	}
}

func TestWithBillNumberRejectsEmpty(t *testing.T) {
	if _, err := WithBillNumber(staticFixture, ""); err == nil {
		t.Fatal("empty bill number accepted")
	}
}
