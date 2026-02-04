package controller

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"genspark2api/common"
	"genspark2api/common/config"
	logger "genspark2api/common/loggger"
	"genspark2api/model"
	"github.com/deanxv/CycleTLS/cycletls"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"io"
	"net/http"
	"strings"
	"time"
)

func VideosForOpenAI(c *gin.Context) {

	client := cycletls.Init()
	defer safeClose(client)

	var openAIReq model.VideosGenerationRequest
	if err := c.BindJSON(&openAIReq); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if lo.Contains(common.VideoModelList, openAIReq.Model) == false {
		c.JSON(400, gin.H{"error": "Invalid model"})
		return
	}

	resp, err := VideoProcess(c, client, openAIReq)
	if err != nil {
		logger.Errorf(c.Request.Context(), fmt.Sprintf("VideoProcess err  %v\n", err))
		c.JSON(http.StatusInternalServerError, model.OpenAIErrorResponse{
			OpenAIError: model.OpenAIError{
				Message: err.Error(),
				Type:    "request_error",
				Code:    "500",
			},
		})
		return
	} else {
		c.JSON(200, resp)
	}

}

func VideoProcess(c *gin.Context, client cycletls.CycleTLS, openAIReq model.VideosGenerationRequest) (*model.VideosGenerationResponse, error) {
	const (
		errNoValidCookies = "No valid cookies available"
		errServerErrMsg   = "An error occurred with the current request, please try again"
		errNoValidTaskIDs = "No valid task IDs received"
	)

	var (
		maxRetries int
		cookie     string
		chatId     string
		lastError  string // 记录最后一次错误
	)

	cookieManager := config.NewCookieManager()
	ctx := c.Request.Context()

	// Initialize session manager and get initial cookie
	if len(config.SessionImageChatMap) == 0 {
		//logger.Warnf(ctx, "未配置环境变量 SESSION_IMAGE_CHAT_MAP, 可能会生图失败!")
		maxRetries = len(cookieManager.Cookies)

		var err error
		cookie, err = cookieManager.GetRandomCookie()
		if err != nil {
			logger.Errorf(ctx, "Failed to get initial cookie: %v", err)
			return nil, fmt.Errorf(errNoValidCookies)
		}
	}

	// 只有一个 cookie 时不重试
	if maxRetries <= 1 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Create request body
		requestBody, err := createVideoRequestBody(c, cookie, &openAIReq, chatId)
		if err != nil {
			logger.Errorf(ctx, "Failed to create request body: %v", err)
			return nil, err
		}

		// Marshal request body
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal request body: %v", err)
			return nil, err
		}

		// Make request
		response, err := makeVideoRequest(client, jsonData, cookie)
		if err != nil {
			logger.Errorf(ctx, "Failed to make video request: %v", err)
			return nil, err
		}

		body := response.Body

		switch {
		case common.IsRateLimit(body):
			lastError = fmt.Sprintf("Cookie rate limited, response: %s", body)
			logger.Warnf(ctx, "Cookie rate limited, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, cookie)
			config.AddRateLimitCookie(cookie, time.Now().Add(time.Duration(config.RateLimitCookieLockDuration)*time.Second))
			if maxRetries == 1 {
				return nil, fmt.Errorf("rate limit reached: %s", body)
			}
			cookie, err = cookieManager.GetNextCookie()
			if err != nil {
				logger.Errorf(ctx, "No more valid cookies available after attempt %d", attempt+1)
				return nil, fmt.Errorf("rate limit, no more cookies: %s", lastError)
			}
			continue
		case common.IsFreeLimit(body):
			lastError = fmt.Sprintf("Cookie free rate limited, response: %s", body)
			logger.Warnf(ctx, "Cookie free rate limited, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, cookie)
			config.AddRateLimitCookie(cookie, time.Now().Add(24*60*60*time.Second))
			if maxRetries == 1 {
				return nil, fmt.Errorf("free limit reached: %s", body)
			}
			cookie, err = cookieManager.GetNextCookie()
			if err != nil {
				logger.Errorf(ctx, "No more valid cookies available after attempt %d", attempt+1)
				return nil, fmt.Errorf("free limit, no more cookies: %s", lastError)
			}
			continue
		case common.IsNotLogin(body):
			lastError = fmt.Sprintf("Cookie not login, response: %s", body)
			logger.Warnf(ctx, "Cookie Not Login, attempt %d/%d, COOKIE:%s", attempt+1, maxRetries, cookie)
			if maxRetries == 1 {
				return nil, fmt.Errorf("cookie not login: %s", body)
			}
			cookie, err = cookieManager.GetNextCookie()
			if err != nil {
				logger.Errorf(ctx, "No more valid cookies available after attempt %d", attempt+1)
				return nil, fmt.Errorf("not login, no more cookies: %s", lastError)
			}
			continue
		case common.IsServerError(body):
			logger.Errorf(ctx, "Server error: %s", body)
			return nil, fmt.Errorf("server error: %s", body)
		case common.IsServerOverloaded(body):
			logger.Errorf(ctx, "Server overloaded: %s", body)
			return nil, fmt.Errorf("server overloaded: %s", body)
		}

		projectId, taskIDs := extractVideoTaskIDs(response.Body)
		if len(taskIDs) == 0 {
			logger.Errorf(ctx, "No task IDs in response: %s", response.Body)
			return nil, fmt.Errorf("%s, response: %s", errNoValidTaskIDs, response.Body)
		}

		// Poll for video URLs
		pollResult := pollVideoTaskStatus(c, client, taskIDs, cookie)
		if pollResult.Error != nil {
			logger.Errorf(ctx, "Poll task status error: %v", pollResult.Error)
			return nil, pollResult.Error
		}

		if len(pollResult.VideoURLs) == 0 {
			lastError = fmt.Sprintf("No video URLs received: %s", pollResult.ErrDetail)
			logger.Errorf(ctx, "No video URLs received: %s", pollResult.ErrDetail)
			if maxRetries == 1 {
				return nil, fmt.Errorf("video generation failed: %s", pollResult.ErrDetail)
			}
			continue
		}

		// Create response object
		result := &model.VideosGenerationResponse{
			Created: time.Now().Unix(),
			Data:    make([]*model.VideosGenerationDataResponse, 0, len(pollResult.VideoURLs)),
		}

		// Process video URLs
		for _, url := range pollResult.VideoURLs {
			data := &model.VideosGenerationDataResponse{
				URL:           url,
				RevisedPrompt: openAIReq.Prompt,
			}

			result.Data = append(result.Data, data)
		}

		// Handle successful case
		if len(result.Data) > 0 {
			// Delete temporary session if needed
			if config.AutoDelChat == 1 {
				go func() {
					client := cycletls.Init()
					defer safeClose(client)
					makeDeleteRequest(client, cookie, projectId)
				}()
			}
			return result, nil
		}
	}

	// All retries exhausted
	logger.Errorf(ctx, "All cookies exhausted after %d attempts, last error: %s", maxRetries, lastError)
	if lastError != "" {
		return nil, fmt.Errorf("all attempts failed: %s", lastError)
	}
	return nil, fmt.Errorf("all cookies are temporarily unavailable")
}

func createVideoRequestBody(c *gin.Context, cookie string, openAIReq *model.VideosGenerationRequest, chatId string) (map[string]interface{}, error) {

	// 创建模型配置
	modelConfigs := []map[string]interface{}{
		{
			"model":              openAIReq.Model,
			"aspect_ratio":       openAIReq.AspectRatio,
			"reflection_enabled": openAIReq.AutoPrompt,
			"duration":           openAIReq.Duration,
		},
	}

	// 创建消息数组
	var messages []map[string]interface{}

	if openAIReq.Image != "" {
		var base64Data string

		if strings.HasPrefix(openAIReq.Image, "http://") || strings.HasPrefix(openAIReq.Image, "https://") {
			// 下载文件
			bytes, err := fetchImageBytes(openAIReq.Image)
			if err != nil {
				logger.Errorf(c.Request.Context(), fmt.Sprintf("fetchImageBytes err  %v\n", err))
				return nil, fmt.Errorf("fetchImageBytes err  %v\n", err)
			}

			contentType := http.DetectContentType(bytes)
			if strings.HasPrefix(contentType, "image/") {
				// 是图片类型，转换为base64
				base64Data = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(bytes)
			}
		} else if common.IsImageBase64(openAIReq.Image) {
			// 如果已经是 base64 格式
			if !strings.HasPrefix(openAIReq.Image, "data:image") {
				base64Data = "data:image/jpeg;base64," + openAIReq.Image
			} else {
				base64Data = openAIReq.Image
			}
		}

		// 构建包含图片的消息
		if base64Data != "" {
			messages = []map[string]interface{}{
				{
					"role": "user",
					"content": []map[string]interface{}{
						{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": base64Data,
							},
						},
						{
							"type": "text",
							"text": openAIReq.Prompt,
						},
					},
				},
			}
		}
	}

	// 如果没有图片或处理图片失败，使用纯文本消息
	if len(messages) == 0 {
		messages = []map[string]interface{}{
			{
				"role":    "user",
				"content": openAIReq.Prompt,
			},
		}
	}
	var currentQueryString string
	if len(chatId) != 0 {
		currentQueryString = fmt.Sprintf("id=%s&type=%s", chatId, videoType)
	} else {
		currentQueryString = fmt.Sprintf("type=%s", videoType)
	}

	// 创建请求体
	requestBody := map[string]interface{}{
		"type": "COPILOT_MOA_VIDEO",
		//"current_query_string": "type=COPILOT_MOA_IMAGE",
		"current_query_string": currentQueryString,
		"messages":             messages,
		"user_s_input":         openAIReq.Prompt,
		"action_params":        map[string]interface{}{},
		"extra_data": map[string]interface{}{
			"model_configs": modelConfigs,
			"imageModelMap": map[string]interface{}{},
		},
	}

	logger.Debug(c.Request.Context(), fmt.Sprintf("RequestBody: %v", requestBody))

	if strings.TrimSpace(config.RecaptchaProxyUrl) == "" ||
		(!strings.HasPrefix(config.RecaptchaProxyUrl, "http://") &&
			!strings.HasPrefix(config.RecaptchaProxyUrl, "https://")) {
		return requestBody, nil
	} else {

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}

		// 检查并补充 RecaptchaProxyUrl 的末尾斜杠
		if !strings.HasSuffix(config.RecaptchaProxyUrl, "/") {
			config.RecaptchaProxyUrl += "/"
		}

		// 创建请求
		req, err := http.NewRequest("GET", fmt.Sprintf("%sgenspark", config.RecaptchaProxyUrl), nil)
		if err != nil {
			logger.Errorf(c.Request.Context(), fmt.Sprintf("创建/genspark请求失败   %v\n", err))
			return nil, err
		}

		// 设置请求头
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)

		// 发送请求
		resp, err := client.Do(req)
		if err != nil {
			logger.Errorf(c.Request.Context(), fmt.Sprintf("发送/genspark请求失败   %v\n", err))
			return nil, err
		}
		defer resp.Body.Close()

		// 读取响应体
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Errorf(c.Request.Context(), fmt.Sprintf("读取/genspark响应失败   %v\n", err))
			return nil, err
		}

		type Response struct {
			Code    int    `json:"code"`
			Token   string `json:"token"`
			Message string `json:"message"`
		}

		if resp.StatusCode == 200 {
			var response Response
			if err := json.Unmarshal(body, &response); err != nil {
				logger.Errorf(c.Request.Context(), fmt.Sprintf("读取/genspark JSON 失败   %v\n", err))
				return nil, err
			}

			if response.Code == 200 {
				logger.Debugf(c.Request.Context(), fmt.Sprintf("g_recaptcha_token: %v\n", response.Token))
				requestBody["g_recaptcha_token"] = response.Token
				logger.Infof(c.Request.Context(), fmt.Sprintf("cheat success!"))
				return requestBody, nil
			} else {
				logger.Errorf(c.Request.Context(), fmt.Sprintf("读取/genspark token 失败   %v\n", err))
				return nil, err
			}
		} else {
			logger.Errorf(c.Request.Context(), fmt.Sprintf("请求/genspark失败   %v\n", err))
			return nil, err
		}
	}
}

func makeVideoRequest(client cycletls.CycleTLS, jsonData []byte, cookie string) (cycletls.Response, error) {

	accept := "*/*"

	return client.Do(apiEndpoint, cycletls.Options{
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome",
		Timeout:   10 * 60 * 60,
		Proxy:     config.ProxyUrl, // 在每个请求中设置代理
		Body:      string(jsonData),
		Method:    "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       accept,
			"Origin":       baseURL,
			"Referer":      baseURL + "/",
			"Cookie":       cookie,
			"User-Agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome",
		},
	}, "POST")
}

func extractVideoTaskIDs(responseBody string) (string, []string) {
	var taskIDs []string
	var projectId string

	// 分行处理响应
	lines := strings.Split(responseBody, "\n")
	for _, line := range lines {

		// 找到包含project_id的行
		if strings.Contains(line, "project_start") {
			// 去掉"data: "前缀
			jsonStr := strings.TrimPrefix(line, "data: ")

			// 解析JSON
			var jsonResp struct {
				ProjectID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &jsonResp); err != nil {
				continue
			}

			// 保存project_id
			projectId = jsonResp.ProjectID
		}

		// 找到包含task_id的行
		if strings.Contains(line, "task_id") {
			// 去掉"data: "前缀
			jsonStr := strings.TrimPrefix(line, "data: ")

			// 解析外层JSON
			var outerJSON struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &outerJSON); err != nil {
				continue
			}

			// 解析内层JSON (content字段)
			var innerJSON struct {
				GeneratedVideos []struct {
					TaskID string `json:"task_id"`
				} `json:"generated_videos"`
			}
			if err := json.Unmarshal([]byte(outerJSON.Content), &innerJSON); err != nil {
				continue
			}

			// 提取所有task_id
			for _, img := range innerJSON.GeneratedVideos {
				if img.TaskID != "" {
					taskIDs = append(taskIDs, img.TaskID)
				}
			}
		}
	}
	return projectId, taskIDs
}

// VideoPollTaskResult 包含视频轮询结果和错误信息
type VideoPollTaskResult struct {
	VideoURLs []string
	Error     error
	ErrDetail string
}

func pollVideoTaskStatus(c *gin.Context, client cycletls.CycleTLS, taskIDs []string, cookie string) *VideoPollTaskResult {
	result := &VideoPollTaskResult{
		VideoURLs: []string{},
	}

	requestData := map[string]interface{}{
		"task_ids": taskIDs,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		result.Error = fmt.Errorf("failed to marshal request data: %v", err)
		return result
	}

	sseChan, err := client.DoSSE("https://www.genspark.ai/api/vg_tasks_status", cycletls.Options{
		Timeout: 10 * 60 * 60,
		Proxy:   config.ProxyUrl, // 在每个请求中设置代理
		Body:    string(jsonData),
		Method:  "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "*/*",
			"Origin":       baseURL,
			"Referer":      baseURL + "/",
			"Cookie":       cookie,
			"User-Agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome",
		},
	}, "POST")
	if err != nil {
		logger.Errorf(c, "Failed to make stream request: %v", err)
		result.Error = fmt.Errorf("failed to make stream request: %v", err)
		return result
	}

	var lastData string
	for response := range sseChan {
		if response.Done {
			break
		}

		data := response.Data
		if data == "" {
			continue
		}
		lastData = data

		logger.Debug(c.Request.Context(), strings.TrimSpace(data))

		var responseData map[string]interface{}
		if err := json.Unmarshal([]byte(data), &responseData); err != nil {
			continue
		}

		if responseData["type"] == "TASKS_STATUS_COMPLETE" {
			if finalStatus, ok := responseData["final_status"].(map[string]interface{}); ok {
				for _, taskID := range taskIDs {
					if task, exists := finalStatus[taskID].(map[string]interface{}); exists {
						status, _ := task["status"].(string)
						if status == "SUCCESS" {
							if urls, ok := task["video_urls"].([]interface{}); ok && len(urls) > 0 {
								if videoURL, ok := urls[0].(string); ok {
									result.VideoURLs = append(result.VideoURLs, videoURL)
								}
							}
						} else {
							// 记录失败状态和错误信息
							errMsg, _ := task["error_message"].(string)
							if errMsg == "" {
								errMsg, _ = task["message"].(string)
							}
							result.ErrDetail = fmt.Sprintf("task %s status: %s, error: %s", taskID, status, errMsg)
							logger.Warnf(c.Request.Context(), "Task failed: %s", result.ErrDetail)
						}
					}
				}
			}
		}
	}

	// 如果没有获取到视频URL，记录最后的响应数据
	if len(result.VideoURLs) == 0 && result.ErrDetail == "" {
		result.ErrDetail = fmt.Sprintf("no video URLs in response, last data: %s", lastData)
	}

	return result
}
