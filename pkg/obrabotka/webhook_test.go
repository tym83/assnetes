package obrabotka

import (
	"bytes"
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReplicasShareTheCertificate(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	opts := WebhookOptions{Service: "obrabotchik", Namespace: "assnetes-system"}
	first := &webhookServer{opts: opts, c: c, reader: c}
	second := &webhookServer{opts: opts, c: c, reader: c}
	ca1, cert1, err := first.certificate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ca2, cert2, err := second.certificate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ca1, ca2) || !bytes.Equal(cert1.Certificate[0], cert2.Certificate[0]) {
		t.Fatal("replicas serve different certificates: the API server trusts only one of them")
	}
	if err := cert1.Leaf.VerifyHostname("obrabotchik.assnetes-system.svc"); err != nil {
		t.Fatal(err)
	}
}
