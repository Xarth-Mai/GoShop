-- GoShop PostgreSQL 初始化脚本
-- 与当前 GORM 模型保持一致；可用于空库初始化，也可配合服务启动后的 AutoMigrate 使用。

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash VARCHAR(256) NOT NULL,
    email VARCHAR(128),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    parent_id INTEGER NOT NULL DEFAULT 0,
    name VARCHAR(64) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS spus (
    id SERIAL PRIMARY KEY,
    category_id INTEGER NOT NULL REFERENCES categories(id),
    name VARCHAR(128) NOT NULL,
    subtitle VARCHAR(256),
    description TEXT,
    main_image VARCHAR(512),
    images JSONB,
    detail_html TEXT,
    status INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skus (
    id SERIAL PRIMARY KEY,
    spu_id INTEGER NOT NULL REFERENCES spus(id) ON DELETE CASCADE,
    title VARCHAR(256) NOT NULL,
    specs JSONB,
    price INTEGER NOT NULL,
    stock INTEGER NOT NULL DEFAULT 0,
    sales_volume INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS coupons (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    type INTEGER NOT NULL,
    value INTEGER NOT NULL,
    min_amount INTEGER NOT NULL DEFAULT 0,
    start_time TIMESTAMP WITH TIME ZONE,
    end_time TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_coupons (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    coupon_id INTEGER NOT NULL REFERENCES coupons(id),
    status INTEGER NOT NULL DEFAULT 0,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_coupons_user_id ON user_coupons(user_id);

CREATE TABLE IF NOT EXISTS addresses (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    receiver_name VARCHAR(256) NOT NULL,
    receiver_phone VARCHAR(256) NOT NULL,
    province VARCHAR(64) NOT NULL,
    city VARCHAR(64) NOT NULL,
    district VARCHAR(64) NOT NULL,
    detail_address VARCHAR(512) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_addresses_user_id ON addresses(user_id);

CREATE TABLE IF NOT EXISTS cart_items (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    sku_id INTEGER NOT NULL REFERENCES skus(id),
    quantity INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cart_items_user_id ON cart_items(user_id);

CREATE TABLE IF NOT EXISTS orders (
    id VARCHAR(64) PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    total_amount INTEGER NOT NULL,
    discount_amount INTEGER NOT NULL DEFAULT 0,
    goods_origin_amount INTEGER NOT NULL DEFAULT 0,
    goods_discount_amount INTEGER NOT NULL DEFAULT 0,
    shipping_fee INTEGER NOT NULL DEFAULT 0,
    shipping_discount_amount INTEGER NOT NULL DEFAULT 0,
    tax_fee INTEGER NOT NULL DEFAULT 0,
    tax_discount_amount INTEGER NOT NULL DEFAULT 0,
    payable_amount INTEGER NOT NULL DEFAULT 0,
    status INTEGER NOT NULL DEFAULT 10,
    pay_status INTEGER NOT NULL DEFAULT 0,
    after_sale_status INTEGER NOT NULL DEFAULT 0,
    user_coupon_id INTEGER NOT NULL DEFAULT 0,
    refund_reason VARCHAR(256),
    refund_proof VARCHAR(512),
    receiver_name VARCHAR(256),
    receiver_phone VARCHAR(256),
    receiver_addr VARCHAR(512),
    pay_expire_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);

CREATE TABLE IF NOT EXISTS order_items (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    sku_id INTEGER NOT NULL REFERENCES skus(id),
    price INTEGER NOT NULL,
    quantity INTEGER NOT NULL,
    origin_amount INTEGER NOT NULL DEFAULT 0,
    item_discount_amount INTEGER NOT NULL DEFAULT 0,
    payable_amount INTEGER NOT NULL DEFAULT 0,
    refunded_amount INTEGER NOT NULL DEFAULT 0,
    merchant_id INTEGER NOT NULL DEFAULT 0,
    promotion_snapshot TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);

CREATE TABLE IF NOT EXISTS order_promotion_allocations (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    order_item_id INTEGER REFERENCES order_items(id) ON DELETE CASCADE,
    sku_id INTEGER NOT NULL REFERENCES skus(id),
    campaign_id INTEGER NOT NULL DEFAULT 0,
    user_coupon_id INTEGER NOT NULL DEFAULT 0,
    discount_type INTEGER NOT NULL,
    discount_amount INTEGER NOT NULL,
    allocation_snapshot TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_order_promotion_allocations_order_id ON order_promotion_allocations(order_id);
CREATE INDEX IF NOT EXISTS idx_order_promotion_allocations_user_coupon_id ON order_promotion_allocations(user_coupon_id);

CREATE TABLE IF NOT EXISTS order_state_logs (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status INTEGER,
    to_status INTEGER NOT NULL,
    operator_type INTEGER NOT NULL,
    operator_id INTEGER,
    event VARCHAR(64) NOT NULL,
    remark TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_order_state_logs_order_id ON order_state_logs(order_id);

CREATE TABLE IF NOT EXISTS payment_orders (
    id VARCHAR(64) PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id),
    channel INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    status INTEGER NOT NULL,
    channel_trade_no VARCHAR(128),
    pay_url TEXT,
    idempotency_key VARCHAR(128) UNIQUE,
    paid_at TIMESTAMP WITH TIME ZONE,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_payment_orders_order_id ON payment_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_user_id ON payment_orders(user_id);

CREATE TABLE IF NOT EXISTS payment_transactions (
    id SERIAL PRIMARY KEY,
    payment_order_id VARCHAR(64) NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    channel INTEGER NOT NULL,
    channel_event_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    raw_payload TEXT NOT NULL,
    signature VARCHAR(512),
    process_status INTEGER NOT NULL,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(channel, channel_event_id)
);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_payment_order_id ON payment_transactions(payment_order_id);

CREATE TABLE IF NOT EXISTS refund_orders (
    id VARCHAR(64) PRIMARY KEY,
    payment_order_id VARCHAR(64) NOT NULL REFERENCES payment_orders(id),
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id),
    after_sale_id VARCHAR(64),
    amount INTEGER NOT NULL,
    reason VARCHAR(256),
    status INTEGER NOT NULL,
    channel_refund_no VARCHAR(128),
    idempotency_key VARCHAR(128) UNIQUE,
    refunded_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_refund_orders_order_id ON refund_orders(order_id);

CREATE TABLE IF NOT EXISTS accounting_entries (
    id SERIAL PRIMARY KEY,
    biz_type VARCHAR(64) NOT NULL,
    biz_id VARCHAR(128) NOT NULL,
    account_type VARCHAR(64) NOT NULL,
    direction INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(biz_type, biz_id, account_type, direction)
);

CREATE TABLE IF NOT EXISTS after_sale_orders (
    id VARCHAR(64) PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL REFERENCES orders(id),
    user_id INTEGER NOT NULL REFERENCES users(id),
    type INTEGER NOT NULL,
    status INTEGER NOT NULL,
    reason VARCHAR(256),
    proof_urls TEXT,
    apply_amount INTEGER NOT NULL,
    approved_amount INTEGER NOT NULL DEFAULT 0,
    refund_id VARCHAR(64),
    return_tracking_no VARCHAR(128),
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_after_sale_orders_order_id ON after_sale_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_after_sale_orders_user_id ON after_sale_orders(user_id);

CREATE TABLE IF NOT EXISTS after_sale_items (
    id SERIAL PRIMARY KEY,
    after_sale_id VARCHAR(64) NOT NULL REFERENCES after_sale_orders(id) ON DELETE CASCADE,
    order_item_id INTEGER NOT NULL REFERENCES order_items(id),
    sku_id INTEGER NOT NULL REFERENCES skus(id),
    quantity INTEGER NOT NULL,
    max_refundable_amount INTEGER NOT NULL,
    apply_amount INTEGER NOT NULL,
    approved_amount INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_after_sale_items_after_sale_id ON after_sale_items(after_sale_id);

CREATE TABLE IF NOT EXISTS dead_letter_orders (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL UNIQUE,
    reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (username, password_hash, email) VALUES
('test_user', '$2a$10$tZ276V04s1J3/gO.wIu88.K7e/701h6.6Xj/N3tB5d0QZ3r5lM5Hq', 'test@example.com')
ON CONFLICT (username) DO NOTHING;

INSERT INTO categories (id, name, sort_order) VALUES
(1, '智能手机', 1),
(2, '笔记本', 2),
(3, '穿戴数码', 3),
(4, '智能平板', 4)
ON CONFLICT (id) DO NOTHING;

INSERT INTO spus (id, category_id, name, subtitle, description, main_image, images, detail_html, status) VALUES
(1, 1, 'Claude Phone 1', '懂你的思考伙伴，掌上轻量体验', 'Claude Phone 1 采用极简设计与温暖配色。', 'https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=800&q=80', '["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9"]', '<p>Claude Phone 1 详细介绍内容...</p>', 1),
(2, 2, 'Anthropic Book Pro', '极致性能，为灵感创作而生', '搭载专为大模型应用优化的新一代芯片。', 'https://images.unsplash.com/photo-1496181130204-755241544e3f?auto=format&fit=crop&w=800&q=80', '["https://images.unsplash.com/photo-1496181130204-755241544e3f"]', '<p>Anthropic Book Pro 详细介绍内容...</p>', 1),
(3, 3, 'Artifacts Earbuds', '纯净原音，静享心流时刻', '智能主动降噪耳机 Artifacts Earbuds。', 'https://images.unsplash.com/photo-1505740420928-5e560c06d30e?auto=format&fit=crop&w=800&q=80', '["https://images.unsplash.com/photo-1505740420928-5e560c06d30e"]', '<p>Artifacts Earbuds 详细介绍内容...</p>', 1),
(4, 4, 'Spike Pad Air', '轻薄随行，创意触手可及', 'Spike Pad Air 只有 6.1 毫米的厚度。', 'https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?auto=format&fit=crop&w=800&q=80', '["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0"]', '<p>Spike Pad Air 详细介绍内容...</p>', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO skus (id, spu_id, title, specs, price, stock, sales_volume) VALUES
(1, 1, 'Haiku (128GB)', '{"规格": "128GB / 温暖沙丘"}', 39900, 87, 0),
(2, 1, 'Sonnet (256GB)', '{"规格": "256GB / 珊瑚礁"}', 59900, 50, 0),
(3, 1, 'Opus (512GB)', '{"规格": "512GB / 深邃星空"}', 89900, 20, 0),
(4, 2, 'Haiku Core (16G+512G)', '{"规格": "16GB / 512GB SSD / 银色"}', 899900, 15, 0),
(5, 2, 'Sonnet Core (32G+1T)', '{"规格": "32GB / 1TB SSD / 深空灰"}', 1299900, 10, 0),
(6, 2, 'Opus Core (64G+2T)', '{"规格": "64GB / 2TB SSD / 珊瑚金"}', 1899900, 5, 0),
(7, 3, 'Standard Edition', '{"规格": "标准版 / 象牙白"}', 99900, 100, 0),
(8, 3, 'ANC Pro Edition', '{"规格": "降噪旗舰版 / 珊瑚红"}', 149900, 45, 0),
(9, 4, 'WiFi (128GB)', '{"规格": "128GB / 经典灰"}', 459900, 30, 0),
(10, 4, 'Cellular + WiFi (256GB)', '{"规格": "256GB / 蜂窝版 / 沙丘金"}', 559900, 12, 0)
ON CONFLICT (id) DO NOTHING;
