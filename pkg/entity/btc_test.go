package entity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const entityAddr = "bc1qentityaddr000000000000000000000000000"

func esploraHandler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/address/"+entityAddr, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"address":"` + entityAddr + `",
			"chain_stats":{"funded_txo_sum":500000000,"spent_txo_sum":100000000,"tx_count":7},
			"mempool_stats":{"funded_txo_sum":50000000,"spent_txo_sum":0,"tx_count":1}}`))
	})
	mux.HandleFunc("/address/"+entityAddr+"/txs", func(w http.ResponseWriter, _ *http.Request) {
		// tx1: inflow 1.0 BTC from senderA (net +1.0)
		// tx2: outflow: entity spends 2.0, gets 0.5 change → net −1.5 to receiverB
		w.Write([]byte(`[
		 {"txid":"aaaaaaaaaaaaaaaaaaaaaaaa","status":{"confirmed":true,"block_time":1752900000},
		  "vin":[{"prevout":{"scriptpubkey_address":"1SenderAxxxxxxxxxxxxxxxxxxxxxxxxxx","value":100000000}}],
		  "vout":[{"scriptpubkey_address":"` + entityAddr + `","value":100000000}]},
		 {"txid":"bbbbbbbbbbbbbbbbbbbbbbbb","status":{"confirmed":true,"block_time":1752800000},
		  "vin":[{"prevout":{"scriptpubkey_address":"` + entityAddr + `","value":200000000}}],
		  "vout":[{"scriptpubkey_address":"3ReceiverBxxxxxxxxxxxxxxxxxxxxxxxx","value":150000000},
		          {"scriptpubkey_address":"` + entityAddr + `","value":50000000}]}
		]`))
	})
	return mux
}

func TestBTCSummaryAndTransfers(t *testing.T) {
	srv := httptest.NewServer(esploraHandler(t))
	defer srv.Close()

	c, err := NewBTCClientWithBases([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sum, err := c.Summary(ctx, entityAddr)
	if err != nil {
		t.Fatal(err)
	}
	if sum.BalanceBTC != 4.5 { // (5−1) confirmed + 0.5 mempool
		t.Errorf("balance = %v, want 4.5", sum.BalanceBTC)
	}
	if sum.TxCount != 7 {
		t.Errorf("txcount = %d", sum.TxCount)
	}

	trs, err := c.RecentTransfers(ctx, entityAddr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(trs) != 2 {
		t.Fatalf("transfers = %d", len(trs))
	}
	if trs[0].AmountBTC != 1.0 {
		t.Errorf("tx1 net = %v, want +1.0", trs[0].AmountBTC)
	}
	if len(trs[0].Counterparties) != 1 || trs[0].Counterparties[0].Address != "1SenderAxxxxxxxxxxxxxxxxxxxxxxxxxx" {
		t.Errorf("tx1 counterparties = %+v", trs[0].Counterparties)
	}
	if trs[1].AmountBTC != -1.5 {
		t.Errorf("tx2 net = %v, want −1.5", trs[1].AmountBTC)
	}
	if len(trs[1].Counterparties) != 1 || trs[1].Counterparties[0].ValueBTC != 1.5 {
		t.Errorf("tx2 counterparties = %+v", trs[1].Counterparties)
	}
}

func TestBTCClientFailover(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer bad.Close()
	good := httptest.NewServer(esploraHandler(t))
	defer good.Close()

	c, err := NewBTCClientWithBases([]string{bad.URL, good.URL})
	if err != nil {
		t.Fatal(err)
	}
	sum, err := c.Summary(context.Background(), entityAddr)
	if err != nil {
		t.Fatalf("failover: %v", err)
	}
	if sum.BalanceBTC != 4.5 {
		t.Errorf("failover balance = %v", sum.BalanceBTC)
	}
}

func TestBTCClientAllHostsDown(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	c, err := NewBTCClientWithBases([]string{bad.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Summary(context.Background(), entityAddr); err == nil {
		t.Fatal("expected error when all hosts fail")
	}
}
