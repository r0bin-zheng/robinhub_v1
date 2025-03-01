package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
    modelName  = "deepseek-r1:8b"                      // ollama模型名称
    ollamaURL  = "http://localhost:11434/api/generate" // ollama服务地址
    mongoDBURL = "mongodb://localhost:27017"           // MongoDB地址
)

// 定义MongoDB文档结构（根据实际数据结构调整）
type Document struct {
	Title   string `bson:"title"`
	Content string `bson:"content"`
}

// 新增上传处理函数
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 验证请求方法
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 验证内容类型
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "仅支持JSON格式", http.StatusUnsupportedMediaType)
		return
	}

	// 解析请求体
	var doc Document
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, "请求体解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 基础数据验证
	if doc.Title == "" || doc.Content == "" {
		http.Error(w, "标题和内容不能为空", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取数据库连接
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		http.Error(w, "数据库连接失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Disconnect(ctx)

	// 执行插入操作
	collection := client.Database("mydb").Collection("documents")
	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		http.Error(w, "数据插入失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      res.InsertedID,
		"message": "文档创建成功",
	})
}

func summaryHandler(w http.ResponseWriter, r *http.Request) {
	// 设置10秒超时控制
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 连接MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoDBURL))
	if err != nil {
		http.Error(w, "数据库连接失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Disconnect(ctx)

	// 获取集合并查询文档（示例查询第一个文档）
	collection := client.Database("mydb").Collection("documents")
	var doc Document
	if err := collection.FindOne(ctx, bson.M{}).Decode(&doc); err != nil {
		http.Error(w, "文档查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 构造Ollama请求
	ollamaReq := map[string]interface{}{
		"model":  modelName,       // 使用实际部署的模型名称
		"prompt": fmt.Sprintf("请总结以下内容：\n标题：%s\n内容：%s", doc.Title, doc.Content),
		"stream": false,          // 使用非流式响应简化处理
	}

	jsonData, _ := json.Marshal(ollamaReq)
	
	// 调用Ollama服务
	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		http.Error(w, "AI服务调用失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 解析Ollama响应
	body, _ := io.ReadAll(resp.Body)
	var ollamaResp map[string]interface{}
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		http.Error(w, "AI响应解析失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取总结结果
	summary, ok := ollamaResp["response"].(string)
	if !ok {
		http.Error(w, "无效的AI响应格式", http.StatusInternalServerError)
		return
	}

	// 返回HTTP响应
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(summary))
}

func main() {
    http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/summarize", summaryHandler)
	fmt.Println("HTTP服务已启动，监听端口 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
