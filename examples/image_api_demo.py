"""
图片生成 API 示例 - Python 版本
"""
import requests
import base64
from pathlib import Path

# API 配置
API_BASE = "http://45.127.35.181:7055/v1"
API_KEY = "sk-214ly"

headers = {
    "Content-Type": "application/json",
    "Authorization": f"Bearer {API_KEY}"
}


def text_to_image():
    """文生图示例"""
    print("=== 文生图示例 ===")
    
    response = requests.post(
        f"{API_BASE}/images/generations",
        headers=headers,
        json={
            "model": "nano-banana-pro",
            "prompt": "一只可爱的橘猫坐在月球上，背景是星空",
            "style": "auto",
            "image_size": "2k"
        }
    )
    
    result = response.json()
    print(result)
    
    # 获取图片 URL
    if "data" in result and len(result["data"]) > 0:
        image_url = result["data"][0].get("url")
        print(f"图片地址: {image_url}")
    
    return result


def image_to_image_url():
    """图生图示例 - 使用 URL"""
    print("=== 图生图示例 (URL) ===")
    
    response = requests.post(
        f"{API_BASE}/images/generations",
        headers=headers,
        json={
            "model": "nano-banana-pro",
            "prompt": "把这张图片变成赛博朋克风格",
            "image": "https://example.com/your-image.jpg",
            "style": "auto",
            "image_size": "2k"
        }
    )
    
    result = response.json()
    print(result)
    return result


def image_to_image_base64(image_path: str):
    """图生图示例 - 使用本地图片 Base64"""
    print("=== 图生图示例 (Base64) ===")
    
    # 读取本地图片并转换为 base64
    with open(image_path, "rb") as f:
        image_data = base64.b64encode(f.read()).decode("utf-8")
    
    # 添加 data URI 前缀
    image_base64 = f"data:image/jpeg;base64,{image_data}"
    
    response = requests.post(
        f"{API_BASE}/images/generations",
        headers=headers,
        json={
            "model": "nano-banana-pro",
            "prompt": "把这张图片变成水彩画风格",
            "image": image_base64,
            "style": "auto",
            "image_size": "2k"
        }
    )
    
    result = response.json()
    print(result)
    return result


def get_image_as_base64():
    """获取 base64 格式的图片"""
    print("=== 获取 Base64 格式图片 ===")
    
    response = requests.post(
        f"{API_BASE}/images/generations",
        headers=headers,
        json={
            "model": "nano-banana-pro",
            "prompt": "一朵盛开的玫瑰花",
            "response_format": "b64_json",  # 返回 base64 格式
            "style": "auto",
            "image_size": "2k"
        }
    )
    
    result = response.json()
    
    # 保存为本地文件
    if "data" in result and len(result["data"]) > 0:
        b64_data = result["data"][0].get("b64_json", "")
        if b64_data:
            # 去掉 data URI 前缀
            if "base64," in b64_data:
                b64_data = b64_data.split("base64,")[1]
            
            image_bytes = base64.b64decode(b64_data)
            with open("output.webp", "wb") as f:
                f.write(image_bytes)
            print("图片已保存为 output.webp")
    
    return result


if __name__ == "__main__":
    # 文生图
    text_to_image()
    
    # 图生图 (URL)
    # image_to_image_url()
    
    # 图生图 (本地图片)
    # image_to_image_base64("your-image.jpg")
    
    # 获取 base64 格式图片
    # get_image_as_base64()
