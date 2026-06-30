export interface Sku {
  id: number
  name: string
  price: number
  stock: number
  specs: string
}

export interface Product {
  id: number
  name: string
  subtitle: string
  description: string
  price: number
  image: string
  category: string
  tags: string[]
  skus: Sku[]
}

export const MOCK_PRODUCTS: Product[] = [
  {
    id: 1,
    name: 'Claude Phone 1',
    subtitle: '懂你的思考伙伴，掌上轻量体验',
    description: 'Claude Phone 1 采用极简设计与温暖配色。搭载端侧小模型，无论是日常事务处理还是深度人机对话，都是您最得力的助手。配备超感人像摄像头与低功耗屏幕。',
    price: 39900, // 399.00
    image: 'https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=800&q=80',
    category: '智能手机',
    tags: ['新品', '端侧AI', '长续航'],
    skus: [
      { id: 1, name: 'Haiku (128GB)', price: 39900, stock: 87, specs: '128GB / 温暖沙丘' },
      { id: 2, name: 'Sonnet (256GB)', price: 59900, stock: 50, specs: '256GB / 珊瑚礁' },
      { id: 3, name: 'Opus (512GB)', price: 89900, stock: 20, specs: '512GB / 深邃星空' }
    ]
  },
  {
    id: 2,
    name: 'Anthropic Book Pro',
    subtitle: '极致性能，为灵感创作而生',
    description: '搭载专为大模型应用优化的新一代芯片，Anthropic Book Pro 拥有超凡的运算能力与极长续航。铝合金一体化机身，搭配暖色背光键盘与高刷视网膜屏幕，让每一次敲击都是灵感的碰撞。',
    price: 899900, // 8999.00
    image: 'https://images.unsplash.com/photo-1496181130204-755241544e3f?auto=format&fit=crop&w=800&q=80',
    category: '笔记本',
    tags: ['高效办公', '高分屏', '强劲算力'],
    skus: [
      { id: 4, name: 'Haiku Core (16G+512G)', price: 899900, stock: 15, specs: '16GB / 512GB SSD / 银色' },
      { id: 5, name: 'Sonnet Core (32G+1T)', price: 1299900, stock: 10, specs: '32GB / 1TB SSD / 深空灰' },
      { id: 6, name: 'Opus Core (64G+2T)', price: 1899900, stock: 5, specs: '64GB / 2TB SSD / 珊瑚金' }
    ]
  },
  {
    id: 3,
    name: 'Artifacts Earbuds',
    subtitle: '纯净原音，静享心流时刻',
    description: '智能主动降噪耳机 Artifacts Earbuds，拥有高达 45dB 的宽频深度降噪，配合专研声学单元，为您还原音乐细节。支持头部追踪空间音频，进入您的私密音乐殿堂。',
    price: 99900, // 999.00
    image: 'https://images.unsplash.com/photo-1505740420928-5e560c06d30e?auto=format&fit=crop&w=800&q=80',
    category: '穿戴数码',
    tags: ['智能降噪', '空间音频', '舒适佩戴'],
    skus: [
      { id: 7, name: 'Standard Edition', price: 99900, stock: 100, specs: '标准版 / 象牙白' },
      { id: 8, name: 'ANC Pro Edition', price: 149900, stock: 45, specs: '降噪旗舰版 / 珊瑚红' }
    ]
  },
  {
    id: 4,
    name: 'Spike Pad Air',
    subtitle: '轻薄随行，创意触手可及',
    description: 'Spike Pad Air 只有 6.1 毫米的厚度，极佳的手写笔体验与灵敏的触控反馈，不管是画草图、记笔记还是浏览网页都能轻松胜任。全面兼容手写笔与外接轻薄键盘。',
    price: 459900, // 4599.00
    image: 'https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?auto=format&fit=crop&w=800&q=80',
    category: '智能平板',
    tags: ['轻薄便携', '手写笔支持', '影音娱乐'],
    skus: [
      { id: 9, name: 'WiFi (128GB)', price: 459900, stock: 30, specs: '128GB / 经典灰' },
      { id: 10, name: 'Cellular + WiFi (256GB)', price: 559900, stock: 12, specs: '256GB / 蜂窝版 / 沙丘金' }
    ]
  }
]
