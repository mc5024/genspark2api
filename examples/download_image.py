"""
Genspark 图片下载客户端
用于下载 API 返回的图片 URL
"""
import requests
import os
from datetime import datetime

# ============================================
# 配置区域 - 修改这里
# ============================================
GS_COOKIE = "i18n_set=zh-CN; session_id=2c1bcc3e-d4e6-4f0b-971a-1b548b7f8b6a:696e36970b04c067485e74a43e21ea7b769c23a73f4deee7d01ae8dc8a09c08f; gslogin=1"

# ============================================

def download_image(image_url: str, save_path: str = None) -> str:
    """
    下载 Genspark 图片
    
    Args:
        image_url: Genspark 图片 URL
        save_path: 保存路径，不传则自动生成
    
    Returns:
        保存的文件路径
    """
    headers = {
        "Cookie": GS_COOKIE,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
    }
    
    resp = requests.get(image_url, headers=headers, timeout=60)
    resp.raise_for_status()
    
    if save_path is None:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        save_path = f"image_{timestamp}.webp"
    
    with open(save_path, "wb") as f:
        f.write(resp.content)
    
    print(f"图片已保存: {save_path} ({len(resp.content)} bytes)")
    return save_path


def generate_and_download(api_base: str, api_key: str, prompt: str, model: str = "nano-banana-pro"):
    """
    生成图片并下载
    
    Args:
        api_base: API 地址，如 http://localhost:7055/v1
        api_key: API 密钥
        prompt: 图片描述
        model: 模型名称
    """
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}"
    }
    
    print(f"正在生成图片: {prompt}")
    resp = requests.post(
        f"{api_base}/images/generations",
        headers=headers,
        json={
            "model": model,
            "prompt": prompt,
            "style": "auto",
            "image_size": "2k"
        },
        timeout=120
    )
    resp.raise_for_status()
    
    result = resp.json()
    print(f"生成完成，共 {len(result.get('data', []))} 张图片")
    
    for i, item in enumerate(result.get("data", [])):
        url = item.get("url")
        if url:
            save_path = f"generated_{i+1}.webp"
            download_image(url, save_path)


if __name__ == "__main__":
    import sys
    
    if len(sys.argv) > 1:
        # 命令行模式：传入 URL 直接下载
        url = sys.argv[1]
        save_path = sys.argv[2] if len(sys.argv) > 2 else None
        download_image(url, save_path)
    else:
        # 示例：生成并下载
        generate_and_download(
            api_base="http://localhost:7055/v1",
            api_key="sk-214ly",
            prompt="一只可爱的橘猫"
        )
