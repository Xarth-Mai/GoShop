<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { MOCK_PRODUCTS } from '../api/mockData'
import Card from '../components/ui/Card.vue'
import Badge from '../components/ui/Badge.vue'
import Button from '../components/ui/Button.vue'

const router = useRouter()
const categories = ['全部', '智能手机', '笔记本', '穿戴数码', '智能平板']
const activeCategory = ref('全部')

const filteredProducts = computed(() => {
  if (activeCategory.value === '全部') {
    return MOCK_PRODUCTS
  }
  return MOCK_PRODUCTS.filter(p => p.category === activeCategory.value)
})

const handleViewProduct = (id: number) => {
  router.push(`/product/${id}`)
}
</script>

<template>
  <div class="home-container">
    <!-- Hero Section -->
    <section class="hero-section">
      <div class="hero-content">
        <Badge variant="coral" class="hero-badge">2026 夏季新品上线</Badge>
        <h1 class="typography-display-xl hero-title">智能生活的<br/>温度与力量</h1>
        <p class="typography-body-md hero-subtitle">
          GoShop 带来全新温和美学设计与企业级高并发底座。这不仅是一个购物平台，更是每一次顺畅交互、极致原子化库存扣减背后的技术结晶。
        </p>
        <div class="hero-actions">
          <Button @click="activeCategory = '智能手机'" variant="primary">立即选购</Button>
          <a href="https://shop.lzzz.ink/swagger/index.html" target="_blank" class="tech-doc-link">
            技术文档
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
              <polyline points="15 3 21 3 21 9"></polyline>
              <line x1="10" y1="14" x2="21" y2="3"></line>
            </svg>
          </a>
        </div>
      </div>
      <div class="hero-visual">
        <div class="hero-card">
          <div class="hero-card-header">
            <span class="status-dot"></span>
            <span>Valkey Node Active</span>
          </div>
          <div class="hero-card-body">
            <span class="tech-metric-title">系统秒杀吞吐量 (QPS)</span>
            <div class="tech-metric-value">12,850+</div>
            <p class="tech-metric-desc">利用 Lua 脚本扣减缓存库存，从容应对百万级瞬时高流量冲击。</p>
          </div>
        </div>
      </div>
    </section>

    <!-- Product Filter Tabs -->
    <section class="filter-section">
      <div class="tabs-container">
        <button
          v-for="cat in categories"
          :key="cat"
          @click="activeCategory = cat"
          :class="['category-tab', { active: activeCategory === cat }]"
        >
          {{ cat }}
        </button>
      </div>
    </section>

    <!-- Product Grid -->
    <section class="products-section">
      <div class="products-grid">
        <Card
          v-for="product in filteredProducts"
          :key="product.id"
          variant="cream"
          :hoverable="true"
          class="product-item"
          @click="handleViewProduct(product.id)"
        >
          <div class="product-image-wrapper">
            <img :src="product.image" :alt="product.name" class="product-image" />
            <div class="product-tag-row">
              <Badge v-for="tag in product.tags" :key="tag" variant="pill" class="product-tag">
                {{ tag }}
              </Badge>
            </div>
          </div>
          <div class="product-info">
            <h3 class="product-title">{{ product.name }}</h3>
            <p class="product-subtitle">{{ product.subtitle }}</p>
            <div class="product-footer">
              <span class="product-price">¥{{ (product.price / 100).toFixed(2) }}</span>
              <span class="view-detail-btn">
                购买
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="5" y1="12" x2="19" y2="12"></line>
                  <polyline points="12 5 19 12 12 19"></polyline>
                </svg>
              </span>
            </div>
          </div>
        </Card>
      </div>
    </section>
  </div>
</template>

<style scoped>
.home-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 var(--spacing-lg) var(--spacing-section) var(--spacing-lg);
  width: 100%;
}

/* Hero Section */
.hero-section {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr;
  align-items: center;
  gap: var(--spacing-xxl);
  padding: var(--spacing-section) 0;
}

@media (max-width: 768px) {
  .hero-section {
    grid-template-columns: 1fr;
    gap: var(--spacing-xl);
    padding: var(--spacing-xl) 0;
    text-align: center;
  }
}

.hero-content {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--spacing-md);
}

@media (max-width: 768px) {
  .hero-content {
    align-items: center;
  }
}

.hero-badge {
  margin-bottom: var(--spacing-xs);
}

.hero-title {
  margin-bottom: var(--spacing-xs);
}

.hero-subtitle {
  color: var(--colors-body);
  max-width: 540px;
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
  margin-top: var(--spacing-md);
}

.tech-doc-link {
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-muted);
  display: flex;
  align-items: center;
  gap: 4px;
}

.tech-doc-link:hover {
  color: var(--colors-ink);
  text-decoration: none;
}

.hero-visual {
  display: flex;
  justify-content: center;
}

.hero-card {
  background-color: var(--colors-surface-dark);
  color: var(--colors-on-dark);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-xl);
  width: 100%;
  max-width: 360px;
  box-shadow: 0 10px 30px rgba(24, 23, 21, 0.15);
}

.hero-card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 12px;
  color: var(--colors-on-dark-soft);
  margin-bottom: var(--spacing-lg);
}

.status-dot {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: 50%;
  display: inline-block;
  box-shadow: 0 0 8px var(--colors-success);
}

.tech-metric-title {
  font-size: 13px;
  color: var(--colors-on-dark-soft);
  display: block;
}

.tech-metric-value {
  font-family: var(--font-serif);
  font-size: 48px;
  margin: var(--spacing-xs) 0;
  color: var(--colors-primary);
}

.tech-metric-desc {
  font-size: 13px;
  color: var(--colors-on-dark-soft);
  line-height: 1.5;
}

/* Filter Section */
.filter-section {
  padding: var(--spacing-lg) 0;
  border-bottom: 1px solid var(--colors-hairline);
  margin-bottom: var(--spacing-xl);
}

.tabs-container {
  display: flex;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.category-tab {
  background: transparent;
  border: none;
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-muted);
  padding: 8px 16px;
  border-radius: var(--rounded-md);
  transition: all 0.2s ease;
}

.category-tab:hover {
  color: var(--colors-ink);
  background-color: var(--colors-surface-soft);
}

.category-tab.active {
  color: var(--colors-ink);
  background-color: var(--colors-surface-card);
}

/* Products Section */
.products-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-lg);
}

@media (max-width: 1024px) {
  .products-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .products-grid {
    grid-template-columns: 1fr;
  }
}

.product-item {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.product-image-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 4/3;
  overflow: hidden;
  border-radius: var(--rounded-md);
  margin-bottom: var(--spacing-md);
  background-color: var(--colors-surface-soft);
}

.product-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.4s cubic-bezier(0.16, 1, 0.3, 1);
}

.product-item:hover .product-image {
  transform: scale(1.05);
}

.product-tag-row {
  position: absolute;
  top: var(--spacing-sm);
  left: var(--spacing-sm);
  display: flex;
  gap: var(--spacing-xxs);
}

.product-info {
  display: flex;
  flex-direction: column;
  flex-grow: 1;
  gap: var(--spacing-xs);
}

.product-title {
  font-family: var(--font-serif);
  font-size: 22px;
  font-weight: 500;
  color: var(--colors-ink);
}

.product-subtitle {
  font-size: 14px;
  color: var(--colors-muted);
  line-height: 1.4;
  flex-grow: 1;
}

.product-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: var(--spacing-md);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--colors-hairline-soft);
}

.product-price {
  font-family: var(--font-sans);
  font-size: 18px;
  font-weight: 600;
  color: var(--colors-ink);
}

.view-detail-btn {
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-primary);
  display: flex;
  align-items: center;
  gap: 4px;
  transition: transform 0.2s ease;
}

.product-item:hover .view-detail-btn {
  transform: translateX(3px);
}
</style>
