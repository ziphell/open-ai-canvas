package skills

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

const (
	skillStatusEnabled = 1
	skillSourceUser    = 1
)

var ErrNotBuiltinSkill = errors.New("target is not a builtin skill")

var skillCategoryLabels = map[string]string{
	"drama":     "短剧影视",
	"ecommerce": "电商营销",
	"creative":  "创意设计",
	"social":    "社媒内容",
	"others":    "其他",
}

type SkillShowcaseMedia struct {
	Type        string `json:"type"`
	ShowcaseURI string `json:"showcaseUri"`
	ShowcaseURL string `json:"showcaseUrl"`
}

type SkillEffectiveUser struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	UID       string `json:"uid"`
}

type SkillItem struct {
	SkillID         string               `json:"skillId"`
	SkillName       string               `json:"skillName"`
	Description     string               `json:"description"`
	Instruction     string               `json:"instruction,omitempty"`
	VersionID       string               `json:"versionId"`
	Version         string               `json:"version"`
	ContentHash     string               `json:"contentHash"`
	FileCount       int                  `json:"fileCount"`
	TotalBytes      int64                `json:"totalBytes"`
	SourceType      string               `json:"sourceType"`
	SourceURL       string               `json:"sourceUrl"`
	SourceRef       string               `json:"sourceRef"`
	SourceSubdir    string               `json:"sourceSubdir"`
	SourceCommit    string               `json:"sourceCommit"`
	SyncStatus      string               `json:"syncStatus"`
	SyncError       string               `json:"syncError,omitempty"`
	AutoUpdate      bool                 `json:"autoUpdate"`
	LastCheckedAt   *time.Time           `json:"lastCheckedAt,omitempty"`
	LastSyncedAt    *time.Time           `json:"lastSyncedAt,omitempty"`
	Status          int                  `json:"status"`
	MarkdownURL     string               `json:"markdownUrl"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	Source          int                  `json:"source"`
	Tag             string               `json:"tag"`
	SortWeight      int                  `json:"sortWeight"`
	IsPrivate       bool                 `json:"isPrivate"`
	LikeCount       int64                `json:"likeCount"`
	IsLike          bool                 `json:"isLike"`
	OwnerUID        string               `json:"ownerUid"`
	EffectiveUser   SkillEffectiveUser   `json:"effectiveUser"`
	OriginalSkillID *string              `json:"originalSkillId"`
	ShowcaseMedia   []SkillShowcaseMedia `json:"showcaseMedia"`
	AddedCount      int64                `json:"addedCount"`
	IsTest          bool                 `json:"isTest"`
	ExtraInfo       string               `json:"extraInfo"`
	IsAdded         bool                 `json:"isAdded"`
	IsOwner         bool                 `json:"isOwner"`
	// FollowUpdates 表示从广场安装且尚未编辑，可以拉取广场最新内容。
	FollowUpdates  bool `json:"followUpdates"`
	FollowMetadata bool `json:"followMetadata"`
	// HasUpdate 表示来源广场技能的内容已经变化，存在可拉取的更新。
	HasUpdate         bool   `json:"hasUpdate"`
	LibraryCategoryID string `json:"libraryCategoryId,omitempty"`
	// AgentMode 只对已安装或自建的技能有意义：auto 允许 Agent 自动发现，manual 只在点名或固定时使用。
	AgentMode string `json:"agentMode,omitempty"`
}

// AddedSkillReference is the small contract used by the runtime catalog.
// The full SkillItem contains editor, sync, author and showcase fields that
// are only needed by the skill detail/market pages, not when selecting an
// installed skill for a generation request.
type AddedSkillReference struct {
	SkillID     string `json:"skillId"`
	SkillName   string `json:"skillName"`
	Description string `json:"description"`
	VersionID   string `json:"versionId"`
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	IsLike      bool   `json:"isLike"`
	IsAdded     bool   `json:"isAdded"`
	IsOwner     bool   `json:"isOwner"`
	AgentMode   string `json:"agentMode,omitempty"`
}

type SkillCategory struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type SkillListRequest struct {
	PlatformRootID        string
	PlatformCategoryID    string
	PlatformUncategorized bool
	Page                  int
	PageSize              int
	Scope                 string
	Search                string
	Tag                   string
	LibraryCategoryID     string
	LibraryUncategorized  bool
	Sort                  string
}

type SkillList struct {
	Skills     []SkillItem     `json:"skills"`
	TotalCount int64           `json:"totalCount"`
	HasMore    bool            `json:"hasMore"`
	NextOffset int             `json:"nextOffset"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	Categories []SkillCategory `json:"categories"`
}

type SkillMutationRequest struct {
	SkillName     string               `json:"skillName"`
	Description   string               `json:"description"`
	Instruction   string               `json:"instruction"`
	Tag           string               `json:"tag"`
	IsPrivate     bool                 `json:"isPrivate"`
	MarkdownURL   string               `json:"markdownUrl"`
	ShowcaseMedia []SkillShowcaseMedia `json:"showcaseMedia"`
	ExtraInfo     string               `json:"extraInfo"`
}

func (s *Service) Skills(userID string, req SkillListRequest) (*SkillList, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	req = normalizeSkillListRequest(req)
	if req.PlatformRootID != "" || req.PlatformCategoryID != "" || req.PlatformUncategorized {
		state, err := s.repo.SkillCuration(false)
		if err != nil {
			return nil, err
		}
		if !state.Enabled {
			req.PlatformRootID, req.PlatformCategoryID, req.PlatformUncategorized = "", "", false
		}
	}
	if req.LibraryCategoryID != "" && req.LibraryUncategorized {
		return nil, kernel.BadAuthRequest("技能库分类与未分类筛选不能同时使用")
	}
	if req.LibraryCategoryID != "" || req.LibraryUncategorized {
		if req.Scope != "mine" && req.Scope != "created" {
			return nil, kernel.BadAuthRequest("用户技能分类仅支持筛选我的技能或我创建的技能")
		}
	}
	limit, offset := req.PageSize, (req.Page-1)*req.PageSize
	items := make([]SkillItem, 0)
	var total int64
	switch req.Scope {
	case "mine", "created":
		rows, count, err := s.repo.UserInstalledSkills(repository.UserInstalledSkillFilter{
			UserID: userID, CreatedOnly: req.Scope == "created", Search: req.Search,
			LibraryCategoryID: req.LibraryCategoryID, LibraryUncategorized: req.LibraryUncategorized,
			Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		total = count
		for index := range rows {
			item, err := s.userSkillItem(userID, &rows[index], false)
			if err != nil {
				return nil, err
			}
			items = append(items, *item)
		}
	default:
		rows, count, err := s.repo.MarketSkills(repository.MarketSkillFilter{
			UserID: userID, Search: req.Search, Tag: req.Tag,
			PlatformRootID: req.PlatformRootID, PlatformCategoryID: req.PlatformCategoryID, PlatformUncategorized: req.PlatformUncategorized,
			FavoritesOnly: req.Scope == "favorites", Sort: req.Sort, Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		total = count
		for _, row := range rows {
			item, err := s.marketSkillItem(userID, row, false)
			if err != nil {
				return nil, err
			}
			items = append(items, *item)
		}
	}
	nextOffset := req.Page * req.PageSize
	if int64(nextOffset) >= total {
		nextOffset = 0
	}
	return &SkillList{
		Skills:     items,
		TotalCount: total,
		HasMore:    int64(req.Page*req.PageSize) < total,
		NextOffset: nextOffset,
		Page:       req.Page,
		PageSize:   req.PageSize,
		Categories: skillCategories(),
	}, nil
}

func (s *Service) AddedSkills(userID string) ([]AddedSkillReference, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	rows, err := s.repo.UserInstalledSkillsAll(userID)
	if err != nil {
		return nil, err
	}
	references := make([]AddedSkillReference, 0, len(rows))
	for index := range rows {
		item, err := s.userSkillItem(userID, &rows[index], false)
		if err != nil {
			return nil, err
		}
		references = append(references, AddedSkillReference{
			SkillID: item.SkillID, SkillName: item.SkillName, Description: item.Description,
			VersionID: item.VersionID, Version: item.Version, Tag: item.Tag,
			IsLike: item.IsLike, IsAdded: true, IsOwner: item.IsOwner, AgentMode: item.AgentMode,
		})
	}
	return references, nil
}

func (s *Service) SkillDetail(userID string, id string) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if instance, err := s.repo.UserInstalledSkill(userID, id); err != nil {
		return nil, err
	} else if instance != nil {
		return s.userSkillItem(userID, instance, true)
	}
	if row, err := s.repo.MarketSkill(userID, id); err != nil {
		return nil, err
	} else if row != nil {
		return s.marketSkillItem(userID, *row, true)
	}
	return nil, kernel.NotFound("技能不存在或未发布")
}

func (s *Service) CreateSkill(userID string, req SkillMutationRequest) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if strings.TrimSpace(req.Instruction) == "" {
		return nil, kernel.BadAuthRequest("技能正文不能为空")
	}
	archive, err := archiveFromMarkdown([]byte(req.Instruction), req.SkillName, req.Description)
	if err != nil {
		return nil, err
	}
	instance, err := s.createPrivateUserSkill(userID, archive, "markdown", req.MarkdownURL, "", "", "", false, req.SkillName, req.Description)
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, instance, true)
}

func (s *Service) UpdateSkill(userID string, id string, req SkillMutationRequest) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	instance, err := s.repo.UserInstalledSkill(userID, id)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, kernel.NotFound("技能不存在")
	}
	name := strings.TrimSpace(req.SkillName)
	if name == "" {
		name = instance.Name
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = instance.Description
	}
	if utf8.RuneCountInString(name) > 80 || utf8.RuneCountInString(description) > 500 {
		return nil, kernel.BadAuthRequest("技能名称或简介超过长度限制")
	}
	now := time.Now()
	fields := map[string]any{"updated_at": now}
	if instance.SourceSkillID == nil {
		fields["name"] = name
		fields["description"] = description
	} else {
		fields["name_override"] = name
		fields["description_override"] = description
		fields["follow_metadata"] = false
		fields["follow_updates"] = false
	}
	if strings.TrimSpace(req.Instruction) != "" {
		snapshot, snapshotErr := s.instanceSnapshot(instance)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		entries, readErr := readSkillContentEntries(snapshot.content)
		if readErr != nil {
			return nil, readErr
		}
		currentEntry := string(entries["SKILL.md"])
		if currentEntry == req.Instruction {
			// 正文未变化，只更新名称/简介，避免把多文件技能包压成单文件。
			if err := s.repo.UpdateUserInstalledSkill(instance.ID, fields); err != nil {
				return nil, err
			}
			updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
			if err != nil {
				return nil, err
			}
			return s.userSkillItem(userID, updated, true)
		}
		entries["SKILL.md"] = []byte(req.Instruction)
		metadata := parseSkillPackageMetadata(entries["SKILL.md"])
		if metadata.Name == "" {
			metadata.Name = name
		}
		if metadata.Description == "" {
			metadata.Description = description
		}
		archive, archiveErr := finalizeSkillArchive(entries, metadata)
		if archiveErr != nil {
			return nil, archiveErr
		}
		contentID := kernel.NewID()
		data, encodeErr := encodeSkillArchive(archive.Files)
		if encodeErr != nil {
			return nil, encodeErr
		}
		content := &model.SkillContent{ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data, CreatedAt: now}
		files := s.contentFileRecords(contentID, archive.Files)
		fields["content_id"] = contentID
		fields["content_hash"] = archive.ContentHash
		fields["version_number"] = gorm.Expr("version_number + 1")
		fields["version_label"] = editedSkillVersionLabel()
		fields["follow_updates"] = false
		fields["follow_metadata"] = false
		fields["file_count"] = len(archive.Files)
		fields["total_bytes"] = archive.TotalBytes
		if len(archive.Files) > 1 {
			fields["source_type"] = "zip"
		}
		if err := s.repo.ReplaceUserSkillContent(instance.ID, instance.ContentID, fields, content, files); err != nil {
			return nil, err
		}
	} else if err := s.repo.UpdateUserInstalledSkill(instance.ID, fields); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, updated, true)
}

func (s *Service) DeleteSkill(userID string, id string) error {
	if strings.TrimSpace(userID) == "" {
		return kernel.Unauthorized("请先登录")
	}
	if instance, err := s.repo.UserInstalledSkill(userID, id); err != nil {
		return err
	} else if instance != nil {
		return s.repo.DeleteUserInstalledSkill(userID, instance.ID)
	}
	// 兼容迁移前的旧技能行。
	skill, err := s.ownedSkill(userID, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteSkill(skill.ID); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.dataDir, "skill-packages", skill.ID))
}

// DeleteBuiltinSkill 对公共目录技能执行“下架隐藏”，不再硬删除内容和发布记录。
// 已安装用户的快照保留；只有用户自己的技能实例才会真正删除。
func (s *Service) DeleteBuiltinSkill(actor *model.User, id string) error {
	if err := s.requireSkillAdmin(actor); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return kernel.BadAuthRequest("技能 ID 不能为空")
	}
	skill, err := s.repo.SkillAnyStatus(id)
	if err != nil {
		return kernel.NotFound("技能不存在")
	}
	if err := s.repo.UpdateSkillFields(skill.ID, map[string]any{"status": 0, "updated_at": time.Now()}); err != nil {
		return err
	}
	return s.repo.HideMarketSkillListing(skill.ID)
}

func (s *Service) SetSkillAdded(userID string, id string, added bool) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if added {
		if _, err := s.installMarketSkill(userID, id); err != nil {
			return nil, err
		}
		return s.SkillDetail(userID, id)
	}
	instance, err := s.repo.UserInstalledSkill(userID, id)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		instance, err = s.repo.UserInstalledSkillBySource(userID, id)
		if err != nil {
			return nil, err
		}
	}
	if instance == nil {
		return nil, kernel.NotFound("技能不存在")
	}
	if instance.SourceSkillID == nil {
		return nil, kernel.BadAuthRequest("自己创建的技能始终保留在我的技能中")
	}
	sourceID := *instance.SourceSkillID
	if err := s.repo.DeleteUserInstalledSkill(userID, instance.ID); err != nil {
		return nil, err
	}
	if id != sourceID {
		return s.SkillDetail(userID, sourceID)
	}
	return s.SkillDetail(userID, id)
}

func (s *Service) SetSkillLiked(userID string, id string, liked bool) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	catalogID := id
	if instance, err := s.repo.UserInstalledSkill(userID, id); err != nil {
		return nil, err
	} else if instance != nil && instance.SourceSkillID != nil {
		catalogID = *instance.SourceSkillID
	}
	row, err := s.repo.MarketSkill(userID, catalogID)
	if err != nil {
		return nil, err
	}
	if row == nil && liked {
		return nil, kernel.Forbidden("该技能未上架，暂时不能收藏")
	}
	if err := s.repo.SetUserSkillFavorite(userID, catalogID, liked); err != nil {
		return nil, err
	}
	return s.SkillDetail(userID, id)
}

// 所有详情和关系写入都先经过同一可见性边界，避免私有技能通过加入、收藏或画布接口泄露正文。
func (s *Service) visibleSkill(userID string, id string) (*model.Skill, error) {
	if err := s.RequireAdministrator(userID); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, kernel.BadAuthRequest("技能 ID 不能为空")
	}
	skill, err := s.repo.Skill(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.BadAuthRequest("技能不存在或已删除")
	}
	if err != nil {
		return nil, err
	}
	if skill.IsPrivate && skill.OwnerID != userID {
		return nil, kernel.Forbidden("该技能未公开")
	}
	return skill, nil
}

func (s *Service) ownedSkill(userID string, id string) (*model.Skill, error) {
	skill, err := s.visibleSkill(userID, id)
	if err != nil {
		return nil, err
	}
	if skill.OwnerID != userID {
		return nil, kernel.Forbidden("只有作者可以修改或删除该技能")
	}
	return skill, nil
}

func normalizeSkillListRequest(req SkillListRequest) SkillListRequest {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 80 {
		req.PageSize = 80
	}
	switch req.Scope {
	case "public", "mine", "created", "favorites":
	default:
		req.Scope = "public"
	}
	switch req.Sort {
	case "popular", "new", "updated":
	default:
		req.Sort = "popular"
	}
	req.Search = strings.TrimSpace(req.Search)
	req.Tag = strings.TrimSpace(req.Tag)
	if req.Tag != "" {
		if _, ok := skillCategoryLabels[req.Tag]; !ok {
			req.Tag = ""
		}
	}
	return req
}

func normalizeSkillMutationRequest(req SkillMutationRequest, requireInstruction bool) (SkillMutationRequest, string, error) {
	req.SkillName = strings.TrimSpace(req.SkillName)
	req.Description = strings.TrimSpace(req.Description)
	req.Instruction = strings.TrimSpace(req.Instruction)
	req.Tag = strings.TrimSpace(req.Tag)
	req.MarkdownURL = strings.TrimSpace(req.MarkdownURL)
	req.ExtraInfo = strings.TrimSpace(req.ExtraInfo)
	if req.SkillName == "" || utf8.RuneCountInString(req.SkillName) > 80 {
		return req, "", kernel.BadAuthRequest("技能名称必须为 1-80 个字符")
	}
	if req.Description == "" || utf8.RuneCountInString(req.Description) > 500 {
		return req, "", kernel.BadAuthRequest("技能简介必须为 1-500 个字符")
	}
	if (requireInstruction && req.Instruction == "") || utf8.RuneCountInString(req.Instruction) > 100000 {
		return req, "", kernel.BadAuthRequest("技能指令必须为 1-100000 个字符")
	}
	if _, ok := skillCategoryLabels[req.Tag]; !ok {
		return req, "", kernel.BadAuthRequest("请选择有效的技能分类")
	}
	if req.MarkdownURL != "" && !validSkillURL(req.MarkdownURL) {
		return req, "", kernel.BadAuthRequest("Markdown 地址必须是有效的 HTTP(S) 链接")
	}
	if utf8.RuneCountInString(req.ExtraInfo) > 2000 {
		return req, "", kernel.BadAuthRequest("补充信息不能超过 2000 个字符")
	}
	if len(req.ShowcaseMedia) > 8 {
		return req, "", kernel.BadAuthRequest("展示媒体最多添加 8 个")
	}
	for index := range req.ShowcaseMedia {
		media := &req.ShowcaseMedia[index]
		media.Type = strings.TrimSpace(media.Type)
		media.ShowcaseURI = strings.TrimSpace(media.ShowcaseURI)
		media.ShowcaseURL = strings.TrimSpace(media.ShowcaseURL)
		if media.Type != "image" && media.Type != "video" {
			return req, "", kernel.BadAuthRequest("展示媒体类型仅支持图片或视频")
		}
		if !validSkillURL(media.ShowcaseURL) {
			return req, "", kernel.BadAuthRequest("展示媒体必须填写有效的 HTTP(S) 链接")
		}
		if utf8.RuneCountInString(media.ShowcaseURI) > 500 || utf8.RuneCountInString(media.ShowcaseURL) > 2000 {
			return req, "", kernel.BadAuthRequest("展示媒体地址过长")
		}
	}
	mediaJSON, err := json.Marshal(req.ShowcaseMedia)
	if err != nil {
		return req, "", err
	}
	return req, string(mediaJSON), nil
}

func validSkillURL(value string) bool {
	target, err := url.ParseRequestURI(value)
	return err == nil && (target.Scheme == "http" || target.Scheme == "https") && target.Host != ""
}

func skillCategories() []SkillCategory {
	return []SkillCategory{
		{Value: "drama", Label: skillCategoryLabels["drama"]},
		{Value: "ecommerce", Label: skillCategoryLabels["ecommerce"]},
		{Value: "creative", Label: skillCategoryLabels["creative"]},
		{Value: "social", Label: skillCategoryLabels["social"]},
		{Value: "others", Label: skillCategoryLabels["others"]},
	}
}

// parseShowcaseMedia 读取库内展示媒体 JSON。历史记录可能仍是 snake_case，只在持久化层转换；对外 API 只输出 camelCase。
func parseShowcaseMedia(raw string) ([]SkillShowcaseMedia, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []SkillShowcaseMedia{}, nil
	}
	var items []SkillShowcaseMedia
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, errors.New("技能展示媒体数据格式错误")
	}
	if showcaseMediaHasURL(items) {
		return items, nil
	}
	var legacy []struct {
		Type        string `json:"type"`
		ShowcaseURI string `json:"showcase_uri"`
		ShowcaseURL string `json:"showcase_url"`
	}
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return nil, errors.New("技能展示媒体数据格式错误")
	}
	converted := make([]SkillShowcaseMedia, 0, len(legacy))
	for _, item := range legacy {
		converted = append(converted, SkillShowcaseMedia{Type: item.Type, ShowcaseURI: item.ShowcaseURI, ShowcaseURL: item.ShowcaseURL})
	}
	return converted, nil
}

func showcaseMediaHasURL(items []SkillShowcaseMedia) bool {
	for _, item := range items {
		if strings.TrimSpace(item.ShowcaseURI) != "" || strings.TrimSpace(item.ShowcaseURL) != "" {
			return true
		}
	}
	return len(items) == 0
}
