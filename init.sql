-- 创建用户表
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash VARCHAR(256) NOT NULL,
    email VARCHAR(128),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建 SPU 表（商品基本信息）
CREATE TABLE IF NOT EXISTS spus (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建 SKU 表（商品具体规格单品）
CREATE TABLE IF NOT EXISTS skus (
    id SERIAL PRIMARY KEY,
    spu_id INTEGER REFERENCES spus(id) ON DELETE CASCADE,
    title VARCHAR(256) NOT NULL,
    price INT NOT NULL, -- 以“分”为单位，规避浮点数精度丢失
    stock INT NOT NULL DEFAULT 0, -- 物理数据库库存
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建订单表
CREATE TABLE IF NOT EXISTS orders (
    id VARCHAR(64) PRIMARY KEY, -- 订单唯一识别号
    user_id INTEGER REFERENCES users(id),
    total_amount INT NOT NULL, -- 订单总金额（分）
    status SMALLINT NOT NULL DEFAULT 1, -- 1: 待支付, 2: 已支付, 3: 已取消, 4: 已发货, 5: 已完成
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建订单商品详情表
CREATE TABLE IF NOT EXISTS order_items (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(64) REFERENCES orders(id) ON DELETE CASCADE,
    sku_id INTEGER REFERENCES skus(id),
    price INT NOT NULL, -- 下单时的 SKU 单价
    quantity INT NOT NULL, -- 订购数量
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 插入基础演示数据
-- ==========================================

-- 1. 插入一个测试用户 (密码明文为 "123456" 的 bcrypt 密文)
INSERT INTO users (username, password_hash, email) VALUES 
('test_user', '$2a$10$tZ276V04s1J3/gO.wIu88.K7e/701h6.6Xj/N3tB5d0QZ3r5lM5Hq', 'test@example.com')
ON CONFLICT (username) DO NOTHING;

-- 2. 插入测试 SPU 商品
INSERT INTO spus (id, name, description) VALUES 
(1, 'GoShop 极速机械键盘', '客制化 87 键，超低延迟红轴机械键盘，专为高并发编程而生。'),
(2, 'GoShop 电商纪念版 T 恤', '100% 纯棉定制，印有经典的 Gopher 与 Valkey 联合 LOGO。')
ON CONFLICT (id) DO NOTHING;

-- 3. 插入测试 SKU 细分规格
INSERT INTO skus (id, spu_id, title, price, stock) VALUES 
(1, 1, '87 键客制化红轴键盘-冰晶蓝', 39900, 100),
(2, 1, '87 键客制化红轴键盘-极夜黑', 39900, 50),
(3, 2, '纪念版 T 恤-白色-L码', 9900, 200),
(4, 2, '纪念版 T 恤-黑色-XL码', 9900, 150)
ON CONFLICT (id) DO NOTHING;
