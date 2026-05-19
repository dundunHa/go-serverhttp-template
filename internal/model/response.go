package model

// Response 是所有业务接口的统一返回结构。
//
//	{
//	  "code": 200,
//	  "data": ...,
//	  "msg": "success"
//	}
type Response[T any] struct {
	Code int    `json:"code" doc:"业务状态码，200 表示成功" example:"200"`
	Data T      `json:"data" doc:"业务负载"`
	Msg  string `json:"msg" doc:"业务描述信息，成功时为 success" example:"success"`
}

// Message 是简单的文本响应负载，用于探活、帮助类接口。
type Message struct {
	Message string `json:"message" doc:"响应文本" example:"hello"`
}

const (
	CodeSuccess = 200
	MsgSuccess  = "success"
)

// Success 构造一个成功的统一响应。
func Success[T any](data T) Response[T] {
	return Response[T]{
		Code: CodeSuccess,
		Data: data,
		Msg:  MsgSuccess,
	}
}
