#!/bin/bash

# API 配置
API_BASE="http://localhost:7055/v1"
API_KEY="sk-214ly"

# ============================================
# 文生图示例 (Text to Image)
# ============================================
echo "=== 文生图示例 ==="

curl -X POST "${API_BASE}/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{
    "model": "nano-banana-pro",
    "prompt": "一只可爱的橘猫坐在月球上，背景是星空",
    "style": "auto",
    "image_size": "2k"
  }'

echo -e "\n"

# ============================================
# 图生图示例 (Image to Image) - 使用 URL
# ============================================
echo "=== 图生图示例 (URL) ==="

curl -X POST "${API_BASE}/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{
    "model": "nano-banana-pro",
    "prompt": "把这张图片变成赛博朋克风格",
    "image": "https://example.com/your-image.jpg",
    "style": "auto",
    "image_size": "2k"
  }'

echo -e "\n"

# ============================================
# 图生图示例 (Image to Image) - 使用 Base64
# ============================================
echo "=== 图生图示例 (Base64) ==="

# 如果有本地图片，可以这样转换为 base64:
# IMAGE_BASE64=$(base64 -w 0 your-image.jpg)

curl -X POST "${API_BASE}/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{
    "model": "nano-banana-pro",
    "prompt": "把这张图片变成水彩画风格",
    "image": "data:image/jpeg;base64,/9j/4AAQSkZJRg...(base64数据)",
    "style": "auto",
    "image_size": "2k"
  }'

echo -e "\n"

# ============================================
# 其他模型示例
# ============================================
echo "=== 使用其他模型 ==="

curl -X POST "${API_BASE}/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{
    "model": "gpt-image-1",
    "prompt": "一幅日落时分的海边风景画"
  }'
