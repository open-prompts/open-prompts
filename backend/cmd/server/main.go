package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.uber.org/zap"

	pb "awsome-prompt/backend/api/proto/v1"
	"awsome-prompt/backend/internal/data"
	"awsome-prompt/backend/internal/repository"
	"awsome-prompt/backend/internal/service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

func writeError(w http.ResponseWriter, err error) {
	zap.S().Errorf("Error handling request: %v", err)
	st, ok := status.FromError(err)
	if !ok {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var code int
	switch st.Code() {
	case codes.OK:
		code = http.StatusOK
	case codes.InvalidArgument:
		code = http.StatusBadRequest
	case codes.NotFound:
		code = http.StatusNotFound
	case codes.AlreadyExists:
		code = http.StatusConflict
	case codes.PermissionDenied:
		code = http.StatusForbidden
	case codes.Unauthenticated:
		code = http.StatusUnauthorized
	case codes.Unimplemented:
		code = http.StatusNotImplemented
	case codes.Unavailable:
		code = http.StatusServiceUnavailable
	default:
		code = http.StatusInternalServerError
	}

	http.Error(w, st.Message(), code)
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	zap.S().Info("Starting application...")
	// Database connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/awsome_prompt?sslmode=disable"
	}
	pgConn, err := data.NewPostgresConnection(dsn)
	if err != nil {
		zap.S().Fatalf("failed to connect to database: %v", err)
	}

	// Repository and Service
	templateRepo := repository.NewTemplateRepository(pgConn.DB)
	promptRepo := repository.NewPromptRepository(pgConn.DB)
	templateVersionRepo := repository.NewTemplateVersionRepository(pgConn.DB)

	svc := service.NewPromptService(promptRepo, templateRepo, templateVersionRepo)

	// Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient, err := data.NewRedisClient(redisAddr, "", 0)
	if err != nil {
		zap.S().Fatalf("failed to connect to redis: %v", err)
	}

	// User Service
	userRepo := repository.NewUserRepository(pgConn.DB)
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-secret-key-change-me"
	}

	// Email Service
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = "noreply@awsome-prompt.com"
	}
	emailSvc := service.NewEmailService(smtpHost, smtpPort, smtpUser, smtpPassword, smtpFrom)

	userSvc := service.NewUserService(userRepo, redisClient, emailSvc, jwtSecret)

	// Auth Interceptor
	authInterceptor := service.NewAuthInterceptor(jwtSecret)

	// gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			zap.S().Fatalf("failed to listen: %v", err)
		}

		// Register Interceptor
		s := grpc.NewServer(
			grpc.UnaryInterceptor(authInterceptor.Unary()),
		)

		pb.RegisterPromptServiceServer(s, svc)
		pb.RegisterUserServiceServer(s, userSvc)
		zap.S().Infof("gRPC server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			zap.S().Fatalf("failed to serve: %v", err)
		}
	}()

	// Helper for JSON marshaling
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	// HTTP Server for FVT/REST
	http.HandleFunc("/api/v1/categories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req := &pb.ListCategoriesRequest{}
		if v := r.URL.Query().Get("owner_id"); v != "" {
			req.OwnerId = v
		}
		if v := r.URL.Query().Get("language"); v != "" {
			req.Language = v
		}

		resp, err := svc.ListCategories(context.Background(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req := &pb.ListTagsRequest{}
		if v := r.URL.Query().Get("language"); v != "" {
			req.Language = v
		}

		resp, err := svc.ListTags(context.Background(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		switch r.Method {
		case http.MethodGet:
			// Optional Auth for Mixed View
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyToken(tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				} else {
					// Optional: fail if token matches format but is invalid?
					// For now, let's just log and continue as anonymous (safe fallback)
					// Or strictly fail:
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
			}

			req := &pb.ListTemplatesRequest{}
			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")
			req.OwnerId = q.Get("owner_id")
			req.Category = q.Get("category")
			req.Language = q.Get("language")
			if v := q.Get("visibility"); v != "" {
				switch v {
				case "VISIBILITY_PUBLIC":
					req.Visibility = pb.Visibility_VISIBILITY_PUBLIC
				case "VISIBILITY_PRIVATE":
					req.Visibility = pb.Visibility_VISIBILITY_PRIVATE
				}
			}
			req.Tags = q["tags"] // Supports ?tags=a&tags=b
			if len(req.Tags) == 0 {
				req.Tags = q["tags[]"] // Supports ?tags[]=a&tags[]=b
			}
			if v := q.Get("my_likes"); v == "true" {
				req.MyLikes = true
			}
			if v := q.Get("my_favorites"); v == "true" {
				req.MyFavorites = true
			}

			resp, err := svc.ListTemplates(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPost:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.CreateTemplateRequest // Create Request
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// If client did not provide language, try to derive from Accept-Language header
			if req.Language == "" {
				if al := r.Header.Get("Accept-Language"); al != "" {
					// Take first language tag, then primary subtag
					first := strings.Split(al, ",")[0]
					lang := strings.Split(strings.TrimSpace(first), "-")[0]
					req.Language = strings.ToLower(lang)
				}
			}
			resp, err := svc.CreateTemplate(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/templates/", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/templates/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		if strings.HasSuffix(id, "/fork") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/fork")
			resp, err := svc.ForkTemplate(ctx, templateID)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/versions") {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			templateID := strings.TrimSuffix(id, "/versions")
			req := &pb.ListTemplateVersionsRequest{TemplateId: templateID}

			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")

			resp, err := svc.ListTemplateVersions(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/like") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/like")
			req := &pb.ToggleLikeRequest{TemplateId: templateID}
			resp, err := svc.ToggleLikeTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/favorite") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/favorite")
			req := &pb.ToggleFavoriteRequest{TemplateId: templateID}
			resp, err := svc.ToggleFavoriteTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		switch r.Method {
		case http.MethodGet:
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyToken(tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				}
			}
			req := &pb.GetTemplateRequest{Id: id}
			resp, err := svc.GetTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPut:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.UpdateTemplateRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// If client did not provide language on update, derive from Accept-Language header
			if req.Language == "" {
				if al := r.Header.Get("Accept-Language"); al != "" {
					first := strings.Split(al, ",")[0]
					lang := strings.Split(strings.TrimSpace(first), "-")[0]
					req.Language = strings.ToLower(lang)
				}
			}
			req.TemplateId = id
			resp, err := svc.UpdateTemplate(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodDelete:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyToken(tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			ownerID := r.URL.Query().Get("owner_id")
			req := &pb.DeleteTemplateRequest{Id: id, OwnerId: ownerID}
			resp, err := svc.DeleteTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// User Handlers
	http.HandleFunc("/api/v1/verification-code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.SendVerificationCodeRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.SendVerificationCode(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.RegisterRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.Register(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.LoginRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.Login(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyToken(tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.UpdateProfileRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.Id = userID

			resp, err := userSvc.UpdateProfile(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodGet:
			req := &pb.GetProfileRequest{}
			req.Id = userID

			resp, err := userSvc.GetProfile(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Prompt Handlers
	http.HandleFunc("/api/v1/prompts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			req := &pb.ListPromptsRequest{
				OwnerId:    q.Get("owner_id"),
				TemplateId: q.Get("template_id"),
			}
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")

			resp, err := svc.ListPrompts(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.CreatePromptRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := svc.CreatePrompt(context.Background(), &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/prompts/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			req := &pb.GetPromptRequest{Id: id}
			resp, err := svc.GetPrompt(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodDelete:
			ownerID := r.URL.Query().Get("owner_id")
			req := &pb.DeletePromptRequest{Id: id, OwnerId: ownerID}
			resp, err := svc.DeletePrompt(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	zap.S().Info("HTTP server listening at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		zap.S().Fatalf("failed to serve http: %v", err)
	}
}
