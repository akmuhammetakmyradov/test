package posts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/akmuhammetakmyradov/test/internal/handlers"
	"github.com/akmuhammetakmyradov/test/internal/posts/models"
	"github.com/akmuhammetakmyradov/test/pkg/config"
	"github.com/akmuhammetakmyradov/test/pkg/middlewares"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
	"github.com/jackc/pgx"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	loginURL      = "/login"
	createUserURL = "/create-user"
	createPostURL = "/create-post"
	deletePostURL = "/delete-post"
	readPostsURL  = "/read-posts"
	readPostURL   = "/read-post/:id"
)

type handler struct {
	repository *Repository
	cfg        *config.Configs
}

func NewHandler(repository *Repository, cfg *config.Configs) handlers.Handler {
	return &handler{
		repository: repository,
		cfg:        cfg,
	}
}

func (h *handler) Register(router fiber.Router) {
	router.Post(loginURL, h.LoginHandler)
	router.Post(createUserURL, middlewares.MiddTokenChkUser, middlewares.MiddChkAdmin, h.CreateUserHandler)
	router.Post(createPostURL, middlewares.MiddTokenChkUser, middlewares.MiddChkAdmin, h.CreatePostHandler)
	router.Post(deletePostURL, middlewares.MiddTokenChkUser, middlewares.MiddChkAdmin, h.DeletePostHandler)
	router.Get(readPostsURL, middlewares.MiddTokenChkUser, h.ReadPostsHandler)
	router.Get(readPostURL, middlewares.MiddTokenChkUser, h.ReadPostHandler)
}

func (h *handler) LoginHandler(c *fiber.Ctx) error {
	var inputUser models.LoginDTO

	if err := c.BodyParser(&inputUser); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"errors": err.Error(),
		})
	}

	// check required datas
	if inputUser.Login == "" || inputUser.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "login, password are required",
		})
	}

	data, err := h.repository.Db.GetUser(context.Background(), inputUser.Login)

	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Invalid login",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	err = bcrypt.CompareHashAndPassword([]byte(data.Password), []byte(inputUser.Password))

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid password",
		})
	}

	claim := &jwt.MapClaims{
		"id":   data.ID,
		"type": data.Type,
	}

	accessToken, err := middlewares.TokenEncode(claim, h.cfg.JWT.AccessSecret, h.cfg.JWT.AccessTokenExp)

	if err != nil {
		fmt.Printf("err in auth LoginHandler Token Encode: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":      "successfully logged in",
		"access_token": accessToken,
	})
}

func (h *handler) CreateUserHandler(c *fiber.Ctx) error {
	var user models.UserDTO
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if user.Login == "" || user.Password == "" || user.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "login, password and name are required",
		})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)

	if err != nil {
		fmt.Println("err in posts CreateUserHandler:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	user.Password = string(hashedPassword)

	data, err := h.repository.Db.CreateUser(context.Background(), user)

	if err != nil {
		if strings.Contains(err.Error(), "violates unique constraint") {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"message": "login is already in use",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "user created successfully",
		"data":    data,
	})
}

func (h *handler) CreatePostHandler(c *fiber.Ctx) error {
	ctx := context.Background()
	var post models.PostDTO
	if err := c.BodyParser(&post); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	data, err := h.repository.Db.CreatePost(ctx, post)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	err = h.repository.Cache.Set(ctx, "post:"+fmt.Sprint(data.ID), data, 5*time.Minute)

	if err != nil {
		fmt.Println("err in set cache CreatePostHandler: ", err)
	}

	err = h.repository.Cache.PaginationAdd(ctx, "posts", float64(data.CreatedAt.UnixNano()), data)

	if err != nil {
		fmt.Println("err in set pagination cache CreatePostHandler: ", err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "post created successfully",
		"data":    data,
	})
}

func (h *handler) DeletePostHandler(c *fiber.Ctx) error {
	var postID models.ID
	ctx := context.Background()

	if err := c.BodyParser(&postID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"errors": err.Error(),
		})
	}
	if postID.ID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "id is required",
		})
	}

	err := h.repository.DeletePost(ctx, postID.ID)

	if err != nil {
		if strings.Contains(err.Error(), "not row effected") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "post doesn't exist",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	err = h.repository.Cache.Delete(ctx, "post:"+fmt.Sprint(postID.ID))

	if err != nil {
		fmt.Println("err in DeletePost: ", err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "deleted successfully",
	})
}

func (h *handler) ReadPostsHandler(c *fiber.Ctx) error {
	page, errPage := strconv.Atoi(c.Query("page"))
	limit, errLimit := strconv.Atoi(c.Query("limit"))
	ctx := context.Background()

	if errPage != nil || errLimit != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid query parameter. It should be an integer.",
		})
	}

	cacheData, _ := h.repository.Cache.PaginationGet(ctx, "posts", page, limit)

	if len(cacheData) != 0 {
		jsonData, err := json.Marshal(cacheData)
		if err == nil {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"data": jsonData,
			})
		}
	}

	posts, err := h.repository.GetPosts(ctx, limit, (page-1)*limit)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Sorry something bad happened in server",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": posts,
	})
}

func (h *handler) ReadPostHandler(c *fiber.Ctx) error {
	postID, err := strconv.Atoi(c.Params("id"))
	ctx := context.Background()
	var post models.Post

	if err != nil {
		fmt.Println("err in posts ReadPostHandler:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid query parameter. It should be an integer.",
		})
	}

	cacheValue, err := h.repository.Cache.Get(ctx, fmt.Sprintf("post:%d", postID))

	if err == nil {
		err = json.Unmarshal([]byte(cacheValue), &post)
	}

	if err == redis.Nil || err != nil {
		post, errDb := h.repository.Db.GetPost(ctx, postID)

		if errDb != nil {
			if errDb.Error() == pgx.ErrNoRows.Error() {
				return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
					"message": "post does not exist",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Sorry something bad happened in server",
			})
		}

		if err == redis.Nil {
			errSet := h.repository.Cache.Set(ctx, "post:"+fmt.Sprint(postID), post, 5*time.Minute)

			if errSet != nil {
				fmt.Println("err in set cache CreatePostHandler: ", err)
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": post,
	})
}
