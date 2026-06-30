<template>
  <footer class="footer">
    <div class="footer-container">
      <div class="footer-brand-section">
        <div class="footer-brand">
          <svg class="spike-mark" width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
          </svg>
          <span class="brand-name">GoShop</span>
        </div>
        <p class="brand-desc">
          一个基于 Go 语言与极速 Valkey 内存锁的企业级高性能通用电商平台。
        </p>
      </div>

      <div class="footer-links-grid">
        <div class="footer-col">
          <h4 class="col-title">商品中心</h4>
          <ul>
            <li><a href="#">智能设备</a></li>
            <li><a href="#">穿戴数码</a></li>
            <li><a href="#">精选配件</a></li>
            <li><a href="#">秒杀专区</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h4 class="col-title">技术栈</h4>
          <ul>
            <li><a href="#">Go + Gin</a></li>
            <li><a href="#">Valkey (Redis)</a></li>
            <li><a href="#">PostgreSQL</a></li>
            <li><a href="#">Vue 3 + TS</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h4 class="col-title">帮助中心</h4>
          <ul>
            <li><a href="#">退换货政策</a></li>
            <li><a href="#">配送服务</a></li>
            <li><a href="#">常见问题</a></li>
            <li><a href="#">开发者API</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h4 class="col-title">关于我们</h4>
          <ul>
            <li><a href="#">公司介绍</a></li>
            <li><a href="#">联系合作</a></li>
            <li><a href="#">加入我们</a></li>
            <li><a href="#">服务条款</a></li>
          </ul>
        </div>
      </div>
    </div>
    <div class="footer-bottom">
      <div class="bottom-container">
        <span class="copyright">© 2026 GoShop. All rights reserved. Built with passion and code.</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.footer {
  background-color: var(--colors-surface-dark);
  color: var(--colors-on-dark-soft);
  padding: 64px 0 32px 0;
  border-top: 1px solid var(--colors-surface-dark-soft);
}

.footer-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 var(--spacing-lg);
  display: grid;
  grid-template-columns: 1.5fr 3fr;
  gap: var(--spacing-xxl);
}

@media (max-width: 768px) {
  .footer-container {
    grid-template-columns: 1fr;
    gap: var(--spacing-xl);
  }
}

.footer-brand-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.footer-brand {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--colors-on-dark);
}

.spike-mark {
  color: var(--colors-primary);
}

.brand-name {
  font-family: var(--font-serif);
  font-size: 22px;
  font-weight: 600;
}

.brand-desc {
  font-size: 14px;
  line-height: 1.6;
  max-width: 280px;
}

.footer-links-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-lg);
}

@media (max-width: 768px) {
  .footer-links-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: var(--spacing-md);
  }
}

.footer-col {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.col-title {
  color: var(--colors-on-dark);
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.footer-col ul {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.footer-col a {
  color: var(--colors-on-dark-soft);
  font-size: 13px;
  transition: color 0.2s ease;
}

.footer-col a:hover {
  color: var(--colors-on-dark);
  text-decoration: none;
}

.footer-bottom {
  margin-top: 48px;
  border-top: 1px solid var(--colors-surface-dark-elevated);
  padding-top: 24px;
}

.bottom-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 var(--spacing-lg);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.copyright {
  font-size: 12px;
  color: var(--colors-muted-soft);
}
</style>
