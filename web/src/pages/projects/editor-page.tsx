import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";

import { WorkspaceErrorState, WorkspaceLoadingState } from "@/components/layout/workspace-state";
import { getProject } from "@/services/api/projects";

import ProjectEditorView from "./detail/editor";

/** 独立剪辑工作台路由：按项目读取详情后交给编辑器插件宿主渲染。 */
export default function ProjectEditorPage() {
    const { projectId = "" } = useParams();
    const query = useQuery({
        queryKey: ["project", projectId, "editor"],
        queryFn: () => getProject(projectId),
        enabled: Boolean(projectId),
    });

    if (query.isPending) {
        return (
            <div className="grid h-[100dvh] place-items-center p-6">
                <WorkspaceLoadingState label="正在打开剪辑工作台" detail="读取项目素材与本地时间线" />
            </div>
        );
    }
    if (query.isError || !query.data) {
        return (
            <div className="grid h-[100dvh] place-items-center p-6">
                <WorkspaceErrorState title="剪辑工作台打开失败" description="项目可能已删除，或服务暂时不可用。" onRetry={() => void query.refetch()} />
            </div>
        );
    }

    return (
        <div className="h-[100dvh] min-h-0">
            <ProjectEditorView detail={query.data} />
        </div>
    );
}
