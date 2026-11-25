import { afterAll, afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { Window } from "happy-dom";
import { useUserStore, type LocalUser } from "@/stores/use-user-store";
import { hasSkillLibraryAccess } from "@/lib/skill-access";
import { apiClient } from "@/services/api/request";
import { getSkill, installGitHubSkill, invalidateAddedSkillsCache, listAddedSkills, listSkillPresets } from "@/services/api/skills";
import { createAgentRun } from "@/services/api/agent";

const dom = new Window({ url: "https://skills.example.com/skills" });
const globals = new Map<string, PropertyDescriptor | undefined>();
for (const [key, value] of Object.entries({ window: dom, document: dom.document, navigator: dom.navigator, Event: dom.Event, HTMLElement: dom.HTMLElement, Element: dom.Element, Node: dom.Node })) {
    globals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
}
afterAll(() => {
    dom.happyDOM.abort();
    for (const [key, descriptor] of globals) {
        if (descriptor) Object.defineProperty(globalThis, key, descriptor);
        else Reflect.deleteProperty(globalThis, key);
    }
});

const admin: LocalUser = { id: "skills-access-user", username: "admin", displayName: "Admin", role: "admin", status: "active" };
const ordinary: LocalUser = { ...admin, role: "user" };
const envelope = (data: unknown) => ({ data: { code: 0, data, msg: "ok" }, status: 200, headers: {} });

beforeEach(() => {
    useUserStore.setState({ hydrated: true, user: ordinary });
    invalidateAddedSkillsCache();
});
afterEach(() => {
    useUserStore.setState({ hydrated: false, user: null });
    invalidateAddedSkillsCache();
});

describe("skill library access", () => {
    test("every signed-in user has skill library access", () => {
        expect(hasSkillLibraryAccess()).toBe(true);
        useUserStore.setState({ user: admin });
        expect(hasSkillLibraryAccess()).toBe(true);
        useUserStore.setState({ user: null });
        expect(hasSkillLibraryAccess()).toBe(false);
        useUserStore.setState({ hydrated: false, user: ordinary });
        expect(hasSkillLibraryAccess()).toBe(false);
    });

    test("ordinary users call skill endpoints instead of receiving an admin denial", async () => {
        const request = spyOn(apiClient, "request").mockImplementation(async (config) => {
            if (String(config.url).includes("/skills/added")) return envelope({ skills: [{ skillId: "mine", isAdded: true }] });
            if (String(config.url).includes("/skills/presets")) return envelope({ presets: [{ presetId: "p1" }] });
            if (String(config.url).includes("/skills/install/github")) return envelope({ skill: { skillId: "github", isAdded: true } });
            return envelope({ skill: { skillId: "detail", isAdded: true } });
        });
        try {
            expect((await listAddedSkills()).skills.map((skill) => skill.skillId)).toEqual(["mine"]);
            expect((await listSkillPresets()).presets.map((preset) => preset.presetId)).toEqual(["p1"]);
            expect((await getSkill("detail")).skill.skillId).toBe("detail");
            expect((await installGitHubSkill({ url: "https://github.com/example/skill" })).skill.skillId).toBe("github");
        } finally {
            request.mockRestore();
        }
    });

    test("ordinary Agent requests keep pinned skills and skill context", async () => {
        let body: unknown;
        const request = spyOn(apiClient, "request").mockImplementation((async (config) => {
            body = config.data;
            return envelope({ run: {} });
        }) as typeof apiClient.request);
        const input = { canvasId: "", prompt: "正常对话", idempotencyKey: "permission-test", pinnedSkillIds: ["mine"], contextScope: ["canvas", "skills"] };
        try {
            await createAgentRun(input);
            expect(body).toMatchObject({ pinnedSkillIds: ["mine"], contextScope: ["canvas", "skills"] });
        } finally {
            request.mockRestore();
        }
    });
});
