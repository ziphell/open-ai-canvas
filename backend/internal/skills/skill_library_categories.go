package skills

import (
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

const (
	skillLibraryCategoryPlatform = "platform"
	skillLibraryCategoryPersonal = "personal"
)

type SkillLibraryCategory struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Count int64  `json:"count"`
}

type SkillLibraryCategoryList struct {
	Categories         []SkillLibraryCategory `json:"categories"`
	TotalCount         int64                  `json:"totalCount"`
	UncategorizedCount int64                  `json:"uncategorizedCount"`
}

type SkillLibraryCategoryMutationRequest struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

type SkillLibraryCategoryAssignmentRequest struct {
	CategoryID string `json:"categoryId"`
}

func (s *Service) SkillLibraryCategories(userID string, scope string) (*SkillLibraryCategoryList, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	categories, err := s.repo.UserSkillLibraryCategories(userID)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.UserSkillLibraryCategoryCounts(userID)
	if err != nil {
		return nil, err
	}
	result := &SkillLibraryCategoryList{
		Categories:         make([]SkillLibraryCategory, 0, len(categories)),
		UncategorizedCount: counts[""],
	}
	result.TotalCount = result.UncategorizedCount
	for _, category := range categories {
		count := counts[category.ID]
		result.Categories = append(result.Categories, SkillLibraryCategory{
			ID: category.ID, Name: category.Name, Scope: skillLibraryCategoryPersonal, Count: count,
		})
		result.TotalCount += count
	}
	return result, nil
}

func (s *Service) CreateSkillLibraryCategory(actor *model.User, req SkillLibraryCategoryMutationRequest) (*SkillLibraryCategory, error) {
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if strings.EqualFold(strings.TrimSpace(req.Scope), skillLibraryCategoryPlatform) {
		return nil, kernel.BadAuthRequest("平台分类仅在管理后台维护")
	}
	name, normalizedName := normalizeSkillLibraryCategoryName(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, kernel.BadAuthRequest("分类名称必须为 1-64 个字符")
	}
	now := time.Now()
	category := &model.UserSkillLibraryCategory{
		ID: kernel.NewID(), UserID: actor.ID, Name: name, NormalizedName: normalizedName,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.repo.CreateUserSkillLibraryCategory(category)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, kernel.BadAuthRequest("当前技能库中已存在同名分类")
	}
	return &SkillLibraryCategory{ID: category.ID, Name: category.Name, Scope: skillLibraryCategoryPersonal}, nil
}

func (s *Service) DeleteSkillLibraryCategory(actor *model.User, id string) error {
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return kernel.Unauthorized("请先登录")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return kernel.BadAuthRequest("分类 ID 不能为空")
	}
	category, err := s.repo.UserSkillLibraryCategory(id)
	if err != nil {
		return err
	}
	if category == nil || category.UserID != actor.ID {
		return kernel.NotFound("技能库分类不存在")
	}
	return s.repo.DeleteUserSkillLibraryCategory(actor.ID, id)
}

func (s *Service) SetSkillLibraryCategory(userID string, skillID string, categoryID string) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	instance, err := s.repo.UserInstalledSkill(userID, skillID)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, kernel.NotFound("技能不存在")
	}
	categoryID = strings.TrimSpace(categoryID)
	if categoryID != "" {
		category, err := s.repo.UserSkillLibraryCategory(categoryID)
		if err != nil {
			return nil, err
		}
		if category == nil || category.UserID != userID {
			return nil, kernel.Forbidden("无权使用该技能库分类")
		}
	}
	if err := s.repo.UpdateUserInstalledSkill(instance.ID, map[string]any{"library_category_id": categoryID, "updated_at": time.Now()}); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, updated, false)
}

func normalizeSkillLibraryCategoryName(value string) (string, string) {
	name := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return name, strings.ToLower(name)
}

func normalizeSkillLibraryCategoryListScope(value string) string {
	switch strings.TrimSpace(value) {
	case "created":
		return "created"
	default:
		return "mine"
	}
}
