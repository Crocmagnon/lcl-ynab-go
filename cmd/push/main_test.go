package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
)

//nolint:funlen // mostly test cases in list
func Test_convert(t *testing.T) {
	t.Parallel()

	type args struct {
		reader    io.Reader
		accountID string
	}

	tests := []struct {
		name             string
		args             args
		wantTransactions []Transaction
		wantReconciled   int
		wantErr          bool
	}{
		{
			name:             "nil reader",
			args:             args{nil, "acc-id"},
			wantTransactions: nil,
			wantReconciled:   0,
			wantErr:          false,
		},
		{
			name:             "no transactions",
			args:             args{strings.NewReader(""), "acc-id"},
			wantTransactions: nil,
			wantReconciled:   0,
			wantErr:          false,
		},
		{
			name: "one positive transaction",
			args: args{strings.NewReader(`29/10/2024;80;Virement;;;VIREMENT M JEAN MARTIN OU;;
29/11/2024;100,06;;01234 123456A`), "acc-id"},
			wantTransactions: []Transaction{
				{
					AccountID: "acc-id",
					Date:      "2024-10-29",
					Amount:    80000,
					PayeeName: "VIREMENT M JEAN MARTIN OU",
					Memo:      "VIREMENT M JEAN MARTIN OU",
					Cleared:   "cleared",
					ImportID:  "YNAB:80000:2024-10-29:1",
				},
			},
			wantReconciled: 100060,
			wantErr:        false,
		},
		{
			name: "one negative and one positive transactions",
			args: args{strings.NewReader(`29/10/2024;80;Virement;;;VIREMENT M JEAN MARTIN OU;;
29/10/2024;-21,32;Carte;;CB  MERCH          28/10/24;;0;Divers
29/11/2024;100,06;;01234 123456A`), "acc-id"},
			wantTransactions: []Transaction{
				{
					AccountID: "acc-id",
					Date:      "2024-10-29",
					Amount:    80000,
					PayeeName: "VIREMENT M JEAN MARTIN OU",
					Memo:      "VIREMENT M JEAN MARTIN OU",
					Cleared:   "cleared",
					ImportID:  "YNAB:80000:2024-10-29:1",
				},
				{
					AccountID: "acc-id",
					Date:      "2024-10-28",
					Amount:    -21320,
					PayeeName: "CB  MERCH",
					Memo:      "CB  MERCH          28/10/24",
					Cleared:   "cleared",
					ImportID:  "YNAB:-21320:2024-10-28:1",
				},
			},
			wantReconciled: 100060,
			wantErr:        false,
		},
		{
			name: "same amount same date",
			args: args{strings.NewReader(`29/10/2024;-21,32;Carte;;CB  MERCH1          28/10/24;;0;Divers
29/10/2024;-21,32;Carte;;CB  MERCH2          28/10/24;;0;Divers
29/11/2024;100,06;;01234 123456A`), "acc-id"},
			wantTransactions: []Transaction{
				{
					AccountID: "acc-id",
					Date:      "2024-10-28",
					Amount:    -21320,
					PayeeName: "CB  MERCH1",
					Memo:      "CB  MERCH1          28/10/24",
					Cleared:   "cleared",
					ImportID:  "YNAB:-21320:2024-10-28:1",
				},
				{
					AccountID: "acc-id",
					Date:      "2024-10-28",
					Amount:    -21320,
					PayeeName: "CB  MERCH2",
					Memo:      "CB  MERCH2          28/10/24",
					Cleared:   "cleared",
					ImportID:  "YNAB:-21320:2024-10-28:2",
				},
			},
			wantReconciled: 100060,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, gotReconciled, err := convert(tt.args.reader, tt.args.accountID)
			if (err != nil) != tt.wantErr {
				t.Errorf("convert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.wantTransactions) {
				t.Errorf("convert() got = %v, want %v", got, tt.wantTransactions)
			}

			if gotReconciled != tt.wantReconciled {
				t.Errorf("convert() gotReconciled = %v, want %v", gotReconciled, tt.wantReconciled)
			}
		})
	}
}

func Test_run(t *testing.T) {
	t.Parallel()

	type args struct {
		ctx  context.Context //nolint:containedctx
		args []string
	}

	tests := []struct {
		name       string
		args       args
		wantStdout string
		wantErr    bool
		clientFunc func() *http.Client
	}{
		{
			name: "one positive transaction",
			args: args{
				context.Background(),
				[]string{"-t", "tok", "-b", "bud-id", "-a", "acc", "-f", "./testdata/one-positive.csv"},
			},
			clientFunc: func() *http.Client {
				transport := httpmock.NewMockTransport()
				transport.RegisterResponder(
					http.MethodPost,
					"/v1/budgets/bud-id/transactions",
					httpmock.NewStringResponder(http.StatusOK, `{"data": {"duplicate_import_ids": ["1234"]}}`),
				)

				return &http.Client{Transport: transport}
			},
			wantStdout: `reconciled: 100.06€
successfully pushed 1 transaction(s)
found 1 duplicate(s)
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			client := tt.clientFunc()

			err := run(tt.args.ctx, tt.args.args, stdout, client)
			if (err != nil) != tt.wantErr {
				t.Errorf("run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotStdout := stdout.String(); gotStdout != tt.wantStdout {
				t.Errorf("run() gotStdout = %v, want %v", gotStdout, tt.wantStdout)
			}
		})
	}
}
