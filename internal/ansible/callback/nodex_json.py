from __future__ import annotations

import json

from ansible.plugins.callback import CallbackBase


class CallbackModule(CallbackBase):
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = "stdout"
    CALLBACK_NAME = "nodex_json"

    def __init__(self):
        super().__init__()
        self.plays = []
        self.current_play = None
        self.current_task = None

    def v2_playbook_on_play_start(self, play):
        self.current_play = {"tasks": []}
        self.plays.append(self.current_play)

    def v2_playbook_on_task_start(self, task, is_conditional):
        if self.current_play is None:
            self.current_play = {"tasks": []}
            self.plays.append(self.current_play)
        self.current_task = {"task": {"name": task.get_name()}, "hosts": {}}
        self.current_play["tasks"].append(self.current_task)

    def _record(self, result):
        if self.current_task is None:
            return
        data = result._result
        host = result._host.get_name()
        outcome = {
            "failed": bool(data.get("failed", False)),
            "skipped": bool(data.get("skipped", False)),
            "unreachable": bool(data.get("unreachable", False)),
        }
        if "stdout_lines" in data:
            outcome["stdout_lines"] = data["stdout_lines"]
        if "msg" in data:
            outcome["msg"] = data["msg"]
        if "stat" in data and isinstance(data["stat"], dict):
            outcome["stat"] = {"exists": bool(data["stat"].get("exists", False))}
        self.current_task["hosts"][host] = outcome

    def v2_runner_on_ok(self, result):
        self._record(result)

    def v2_runner_on_changed(self, result):
        self._record(result)

    def v2_runner_on_failed(self, result, ignore_errors=False):
        self._record(result)

    def v2_runner_on_unreachable(self, result):
        self._record(result)

    def v2_runner_on_skipped(self, result):
        self._record(result)

    def v2_playbook_on_stats(self, stats):
        payload = {
            "stats": {
                host: stats.summarize(host)
                for host in sorted(stats.processed)
            },
            "plays": self.plays,
        }
        print(json.dumps(payload, separators=(",", ":")), flush=True)
