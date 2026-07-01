package aftersale

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"GoShop/internal/inventory"
	"GoShop/internal/testutil"
	"GoShop/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPaidOrder(t *testing.T, db *gorm.DB, orderID string, quantity int) models.Order {
	t.Helper()

	userID := uint(1)
	skuID := uint(1)
	price := 39900
	total := price * quantity

	var inv models.SkuInventory
	if err := db.Where("sku_id = ?", skuID).First(&inv).Error; err != nil {
		inv = models.SkuInventory{SkuID: skuID, Available: 100}
		if err := db.Create(&inv).Error; err != nil {
			t.Fatalf("create inventory: %v", err)
		}
	} else {
		inv.Available = 100
		inv.Reserved = 0
		inv.Sold = 0
		if err := db.Save(&inv).Error; err != nil {
			t.Fatalf("reset inventory: %v", err)
		}
	}

	order := models.Order{
		ID:                orderID,
		UserID:            userID,
		TotalAmount:       total,
		PayableAmount:     total,
		GoodsOriginAmount: total,
		Status:            models.OrderStatusPaid,
		PayStatus:         models.PayStatusPaid,
		AfterSaleStatus:   models.AfterSaleStatusNone,
		ReceiverName:      "Test User",
		ReceiverPhone:     "13800138000",
		ReceiverAddr:      "Test Address",
		Items: []models.OrderItem{{
			SkuID:         skuID,
			Price:         price,
			Quantity:      quantity,
			OriginAmount:  total,
			PayableAmount: total,
		}},
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	if err := db.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}

	invSvc := inventory.NewService(db)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := invSvc.ReserveStock(tx, orderID, userID, []inventory.ReserveItem{{SkuID: skuID, Quantity: quantity}}, time.Now().Add(10*time.Minute)); err != nil {
			return err
		}
		return invSvc.ConfirmOrderReservations(tx, orderID)
	}); err != nil {
		t.Fatalf("reserve and confirm stock: %v", err)
	}

	return order
}

func TestAfterSalePartialRefund(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)
	order := setupPaidOrder(t, db, "ORDER-AFTERSALE-PARTIAL", 2)
	item := order.Items[0]

	if err := svc.ApplyRefund(order.UserID, order.ID, ApplyRequest{
		RefundReason: "partial refund",
		Items:        []ApplyItem{{OrderItemID: item.ID, Quantity: 1}},
	}); err != nil {
		t.Fatalf("ApplyRefund failed: %v", err)
	}
	if err := svc.AuditRefund(order.ID, AuditRequest{Action: "approve"}); err != nil {
		t.Fatalf("AuditRefund approve failed: %v", err)
	}
	if err := svc.AuditRefund(order.ID, AuditRequest{Action: "approve"}); err != nil {
		t.Fatalf("duplicate AuditRefund approve should be idempotent: %v", err)
	}

	var updatedOrder models.Order
	if err := db.First(&updatedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("query order: %v", err)
	}
	if updatedOrder.Status != models.OrderStatusPaid || updatedOrder.PayStatus != models.PayStatusPartialRefunded {
		t.Fatalf("expected paid/partial refunded, got status=%d pay=%d", updatedOrder.Status, updatedOrder.PayStatus)
	}

	var updatedItem models.OrderItem
	if err := db.First(&updatedItem, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("query item: %v", err)
	}
	if updatedItem.RefundedAmount != item.PayableAmount/2 {
		t.Fatalf("expected refunded amount %d, got %d", item.PayableAmount/2, updatedItem.RefundedAmount)
	}

	var inv models.SkuInventory
	if err := db.First(&inv, "sku_id = ?", item.SkuID).Error; err != nil {
		t.Fatalf("query inventory: %v", err)
	}
	if inv.Sold != 1 || inv.Available != 99 {
		t.Fatalf("expected sold=1 available=99, got sold=%d available=%d", inv.Sold, inv.Available)
	}
	var event models.OutboxEvent
	if err := db.First(&event, "event_type = ? AND aggregate_type = ?", "RefundSucceeded", "refund").Error; err != nil {
		t.Fatalf("expected RefundSucceeded outbox event: %v", err)
	}
}

func TestAfterSaleFullRefundAndReject(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)

	fullOrder := setupPaidOrder(t, db, "ORDER-AFTERSALE-FULL", 1)
	if err := svc.ApplyRefund(fullOrder.UserID, fullOrder.ID, ApplyRequest{RefundReason: "full refund"}); err != nil {
		t.Fatalf("ApplyRefund full failed: %v", err)
	}
	if err := svc.AuditRefund(fullOrder.ID, AuditRequest{Action: "approve"}); err != nil {
		t.Fatalf("AuditRefund full approve failed: %v", err)
	}
	var refunded models.Order
	if err := db.First(&refunded, "id = ?", fullOrder.ID).Error; err != nil {
		t.Fatalf("query full order: %v", err)
	}
	if refunded.Status != models.OrderStatusRefunded || refunded.PayStatus != models.PayStatusRefunded {
		t.Fatalf("expected full refund status, got status=%d pay=%d", refunded.Status, refunded.PayStatus)
	}

	rejectOrder := setupPaidOrder(t, db, "ORDER-AFTERSALE-REJECT", 1)
	if err := svc.ApplyRefund(rejectOrder.UserID, rejectOrder.ID, ApplyRequest{RefundReason: "reject refund"}); err != nil {
		t.Fatalf("ApplyRefund reject failed: %v", err)
	}
	if err := svc.AuditRefund(rejectOrder.ID, AuditRequest{Action: "reject"}); err != nil {
		t.Fatalf("AuditRefund reject failed: %v", err)
	}
	var rejected models.Order
	if err := db.First(&rejected, "id = ?", rejectOrder.ID).Error; err != nil {
		t.Fatalf("query reject order: %v", err)
	}
	if rejected.Status != models.OrderStatusRefundRejected || rejected.AfterSaleStatus != models.AfterSaleStatusRejected {
		t.Fatalf("expected refund rejected state, got status=%d aftersale=%d", rejected.Status, rejected.AfterSaleStatus)
	}
}

func TestAfterSaleService_IsolatedDB(t *testing.T) {
	// 1. 初始化独立数据库，只做售后相关的 Table Migrate
	dbIsolated, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	err = dbIsolated.AutoMigrate(
		&models.AfterSaleOrder{},
		&models.AfterSaleItem{},
		&models.RefundOrder{},
		&models.AccountingEntry{},
		&models.OutboxEvent{},
		&models.InboxEvent{},
	)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 确认 orders 和 sku_inventories 表不存在
	if dbIsolated.Migrator().HasTable("orders") || dbIsolated.Migrator().HasTable("sku_inventories") {
		t.Fatal("orders and sku_inventories tables should not exist in aftersale isolated database")
	}

	// 2. 启动模拟的订单微服务 HTTP 服务监听 8105 端口，响应 refund-source, refund-apply, refund-complete RPC 请求
	orderListener, err := net.Listen("tcp", "127.0.0.1:8105")
	if err != nil {
		t.Fatalf("Failed to listen on 127.0.0.1:8105: %v", err)
	}
	defer orderListener.Close()

	orderSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/refund-source") {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"orderId":         "ORDER-AS-ISO-01",
					"userId":          1,
					"totalAmount":     39900,
					"status":          20, // Paid
					"payStatus":       20, // Paid
					"afterSaleStatus": 0,
					"items": []map[string]interface{}{
						{
							"orderItemId":        1,
							"skuId":              1,
							"quantity":           1,
							"payableAmount":      39900,
							"refundedAmount":     0,
							"refundableQuantity": 1,
							"refundableAmount":   39900,
						},
					},
				})
				return
			}
			if r.Method == "POST" && (strings.Contains(r.URL.Path, "/refund-apply") || strings.Contains(r.URL.Path, "/refund-complete")) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"success"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	go orderSrv.Serve(orderListener)
	defer orderSrv.Shutdown(context.Background())

	// 3. 启动模拟的库存微服务 HTTP 服务监听 8103 端口，响应 inventory/restock RPC 请求
	invListener, err := net.Listen("tcp", "127.0.0.1:8103")
	if err != nil {
		t.Fatalf("Failed to listen on 127.0.0.1:8103: %v", err)
	}
	defer invListener.Close()

	invSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/inventory/restock") {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"success"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	go invSrv.Serve(invListener)
	defer invSrv.Shutdown(context.Background())

	// 4. 执行售后逻辑，使用售后服务独占的 dbIsolated 连接
	svc := NewService(dbIsolated)

	// 申请售后退款
	err = svc.ApplyRefund(1, "ORDER-AS-ISO-01", ApplyRequest{
		RefundReason: "isolated test refund",
	})
	if err != nil {
		t.Fatalf("ApplyRefund isolated failed: %v", err)
	}

	// 验证售后单已落库至 dbIsolated
	var asOrder models.AfterSaleOrder
	if err := dbIsolated.First(&asOrder, "order_id = ?", "ORDER-AS-ISO-01").Error; err != nil {
		t.Fatalf("failed to query aftersale order: %v", err)
	}
	if asOrder.ApplyAmount != 39900 || asOrder.Status != models.AfterSaleStatusApplying {
		t.Errorf("unexpected aftersale order: %+v", asOrder)
	}

	// 同意售后申请
	err = svc.AuditRefund("ORDER-AS-ISO-01", AuditRequest{Action: "approve"})
	if err != nil {
		t.Fatalf("AuditRefund isolated failed: %v", err)
	}

	// 验证退款单已在 dbIsolated 落库
	var refundOrder models.RefundOrder
	if err := dbIsolated.First(&refundOrder, "order_id = ?", "ORDER-AS-ISO-01").Error; err != nil {
		t.Fatalf("failed to query refund order: %v", err)
	}
	if refundOrder.Amount != 39900 || refundOrder.Status != models.RefundStatusSuccess {
		t.Errorf("unexpected refund order: %+v", refundOrder)
	}
}
