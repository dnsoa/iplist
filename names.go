package iplist

func defaultCloudSet() map[string]bool {
	return map[string]bool{
		"aliyun":       true,
		"tencent":      true,
		"huawei":       true,
		"microsoft":    true,
		"cloudflare":   true,
		"googlecloud":  true,
		"digitalocean": true,
		"bytedance":    true,
		"volcengine":   true,
	}
}

func defaultProviderNames() map[string]string {
	return map[string]string{
		"chinatelecom": "中国电信",
		"chinaunicom":  "中国联通",
		"chinamobile":  "中国移动",
		"drpeng":       "鹏博士",
		"cernet":       "中国教育网",
		"cstnet":       "中国科技网",
		"aliyun":       "阿里云",
		"tencent":      "腾讯云",
		"cloudflare":   "Cloudflare",
		"huawei":       "华为云",
		"microsoft":    "Microsoft",
		"bytedance":    "字节跳动",
		"volcengine":   "火山引擎",
		"googlecloud":  "Google Cloud",
		"digitalocean": "DigitalOcean",
	}
}
