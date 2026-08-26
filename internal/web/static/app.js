const state = { batches: [], detail: null, scopeDigest: "", diagnosisFilter: { point: "", rule: "", outcome: "" } };
const $ = (selector) => document.querySelector(selector);
const makeKey = (operation) => `${operation}-${Date.now()}-${crypto.randomUUID()}`;
const formObject = (root) => root instanceof HTMLFormElement ? Object.fromEntries(new FormData(root).entries()) : Object.fromEntries([...root.querySelectorAll("input, select, textarea")].map((input) => [input.name, input.value]));

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: options.body ? { "Content-Type": "application/json", ...(options.headers || {}) } : options.headers });
  const payload = await response.json().catch(() => ({ error: "服务返回了无法解析的响应" }));
  if (!response.ok) {
    const fields = payload.fields?.map((item) => `${item.field}: ${item.message}`).join("；");
    const error = new Error(fields || payload.error || `请求失败（${response.status}）`);
    error.payload = payload;
    throw error;
  }
  return payload;
}

function showToast(message, isError = false) {
  const toast = $("#toast"); toast.textContent = message; toast.classList.toggle("error", isError); toast.classList.remove("hidden");
  clearTimeout(showToast.timer); showToast.timer = setTimeout(() => toast.classList.add("hidden"), 5200);
}

async function withAction(action, successMessage) {
  try { await action(); showToast(successMessage); }
  catch (error) { showToast(error.message, true); if (error.message.includes("版本冲突") && state.detail) await selectBatch(state.detail.batch.id); }
}

async function loadHealth() {
  try { const health = await api("/api/health"); $("#health").textContent = `证据库正常 · schema v${health.schemaVersion}`; $("#health").classList.add("ok"); }
  catch (error) { $("#health").textContent = `证据库异常：${error.message}`; }
}

async function loadBatches() { const payload = await api("/api/batches"); state.batches = payload.batches || []; renderBatchList(); }

function renderBatchList() {
  const selected = state.detail?.batch.id;
  $("#batchList").innerHTML = state.batches.length ? state.batches.map((batch) => `<button class="batch-item ${batch.id === selected ? "active" : ""}" data-batch-id="${escapeHTML(batch.id)}"><strong>${escapeHTML(batch.cableSection)}</strong><small>${escapeHTML(batch.circuitName)} · ${escapeHTML(batch.statusLabel)}</small><small>v${batch.version} · 未关闭偏差 ${batch.openDeviationCount}</small></button>`).join("") : `<p class="muted">尚无试验批次。</p>`;
  document.querySelectorAll("[data-batch-id]").forEach((button) => button.addEventListener("click", () => selectBatch(button.dataset.batchId)));
}

async function selectBatch(id) {
  state.detail = await api(`/api/batches/${encodeURIComponent(id)}`);
  $("#emptyState").classList.add("hidden"); $("#batchView").classList.remove("hidden"); renderBatch(); renderBatchList();
}

function renderBatch() {
  const { batch, summary, timeline, diagnosisReadiness, reviewReadiness } = state.detail;
  $("#batchID").textContent = batch.id; $("#batchTitle").textContent = `${batch.cableSection} · ${batch.circuitName}`;
  $("#batchMeta").textContent = `试验负责人：${batch.testOwner}${batch.sourceBatchID ? `　复用来源：${batch.sourceBatchID}` : ""}　创建于 ${formatTime(batch.createdAt)}`;
  $("#statusBadge").textContent = summary.statusLabel; $("#versionBadge").textContent = `expectedVersion ${batch.version}`;
  refreshMeasurementPointOptions(); renderMeasurements(batch.measurements || []); renderReadiness(diagnosisReadiness);
  renderReports(batch.diagnosisReports || []); renderDeviations(batch.deviations || []); renderReviewReadiness(reviewReadiness);
  renderReviews(batch.reviews || []); renderCertificate(batch.certificate); renderTimeline(timeline || []); setAvailability(batch);
}

function pointOptions(selected = "") {
  return (state.detail?.batch.frozenScope?.points || []).map((point) => `<option value="${escapeHTML(point.id)}" ${point.id === selected ? "selected" : ""}>${escapeHTML(point.id)} · ${escapeHTML(point.location)}</option>`).join("");
}

function refreshMeasurementPointOptions() {
  document.querySelectorAll("#measurementRows select[name=pointID]").forEach((select) => { const current = select.value; select.innerHTML = pointOptions(current); });
}

function renderMeasurements(items) {
  $("#measurementList").innerHTML = items.length ? items.map((item) => `<div class="evidence"><strong>${escapeHTML(item.pointID)} · 第 ${item.round} 轮 · ${item.purpose === "retest" ? "定向复验" : "初始采样"}</strong><p>峰值 ${item.peakAmplitudePC.toFixed(2)} pC；${escapeHTML(item.phaseSummary)}</p><p class="muted">${formatTime(item.measuredAt)} · ${item.temperatureC} ℃ · 湿度 ${item.humidityPercent}% · ${escapeHTML(item.sensorSerial)}</p><p class="mono">${escapeHTML(item.id)}</p></div>`).join("") : `<p class="muted">尚未录入现场读数。</p>`;
}

function renderReadiness(readiness) {
  if (!readiness) return;
  const blockers = readiness.blockers?.length ? `<ul>${readiness.blockers.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}</ul>` : "";
  const points = (readiness.points || []).map((point) => `<div><strong>${escapeHTML(point.pointID)}</strong>：初始采样 ${point.initialCount}，有效轮次 ${escapeHTML((point.validRounds || []).join(", ") || "无")}，跨度 ${point.timeSpanSeconds}s，传感器${point.sensorConsistent ? "一致" : "不一致"}</div>`).join("");
  $("#diagnosisReadiness").innerHTML = `<strong>${readiness.ready ? "诊断门禁已就绪" : "诊断门禁阻断"}</strong> · 可评估规则 ${readiness.evaluableRuleCount} · 证据不足规则 ${readiness.insufficientRuleCount}${blockers}${points}`;
}

function renderReports(reports) {
  if (!reports.length) { $("#diagnosisReports").innerHTML = `<p class="muted">尚无诊断运行报告。</p>`; return; }
  const latest = reports[reports.length - 1]; const points = [...new Set(latest.results.map((item) => item.pointID))]; const rules = [...new Set(latest.results.map((item) => item.ruleCode))];
  const results = latest.results.filter((item) => (!state.diagnosisFilter.point || item.pointID === state.diagnosisFilter.point) && (!state.diagnosisFilter.rule || item.ruleCode === state.diagnosisFilter.rule) && (!state.diagnosisFilter.outcome || item.outcome === state.diagnosisFilter.outcome));
  $("#diagnosisReports").innerHTML = `<div class="risk"><strong>运行 ${escapeHTML(latest.runID)}</strong> · 证据版本 ${latest.evidenceVersion} · 严重 ${latest.risk.severe} / 主要 ${latest.risk.major} / 提示 ${latest.risk.notice} / 证据不足 ${latest.risk.insufficient}</div><div class="filter-row"><select id="filterPoint"><option value="">全部试验点</option>${points.map((v) => `<option ${v === state.diagnosisFilter.point ? "selected" : ""}>${escapeHTML(v)}</option>`).join("")}</select><select id="filterRule"><option value="">全部规则</option>${rules.map((v) => `<option ${v === state.diagnosisFilter.rule ? "selected" : ""}>${escapeHTML(v)}</option>`).join("")}</select><select id="filterOutcome"><option value="">全部判定</option><option value="passed">通过</option><option value="triggered">触发</option><option value="insufficient">证据不足</option></select></div><div class="table-wrap"><table><thead><tr><th>点位</th><th>规则</th><th>判定</th><th>冻结限值</th><th>实际值</th><th>证据</th><th>说明</th></tr></thead><tbody>${results.map((item) => `<tr><td>${escapeHTML(item.pointID)}</td><td>${escapeHTML(item.ruleCode)}</td><td>${outcomeLabel(item.outcome)}</td><td>${escapeHTML(item.frozenLimit)}</td><td>${escapeHTML(item.actualValue)}</td><td class="mono">${escapeHTML(item.evidenceIDs.join(", "))}</td><td>${escapeHTML(item.explanation)}</td></tr>`).join("")}</tbody></table></div>`;
  $("#filterOutcome").value = state.diagnosisFilter.outcome;
  [["#filterPoint", "point"], ["#filterRule", "rule"], ["#filterOutcome", "outcome"]].forEach(([selector, key]) => $(selector).addEventListener("change", (event) => { state.diagnosisFilter[key] = event.target.value; renderReports(reports); }));
}

function renderDeviations(items) {
  $("#deviationList").innerHTML = items.length ? items.map((item) => { const closed = Boolean(item.closedAt); const task = item.retestTask ? `<div class="task"><strong>复验任务包</strong>：${escapeHTML(item.retestTask.metric)}；${escapeHTML(item.retestTask.frozenLimit)}；需要 ${item.retestTask.requiredRounds} 轮，尚缺 ${item.retestTask.missingRounds} 轮；状态 ${escapeHTML(item.retestTask.status)}${item.retestTask.failureReason ? `；${escapeHTML(item.retestTask.failureReason)}` : ""}</div>` : ""; return `<article class="deviation ${closed ? "closed" : ""}"><div class="deviation-head"><strong>${escapeHTML(item.ruleCode)} · ${escapeHTML(item.location)}</strong><span class="severity">${closed ? "已关闭" : escapeHTML(item.severity)}</span></div><p>${escapeHTML(item.finding)}</p>${item.correction ? `<p>整改：${escapeHTML(item.correction.measure)} · 责任人 ${escapeHTML(item.correction.assignee)}</p>` : ""}${task}${item.retest ? `<p>复验：${escapeHTML(item.retest.conclusion)}（${item.retest.result}）</p>` : ""}<p class="mono">${escapeHTML(item.id)}</p>${closed ? "" : deviationForms(item)}</article>`; }).join("") : `<p class="muted">当前没有诊断偏差。完成读数后运行规则诊断。</p>`;
  bindDeviationForms();
}

function deviationForms(item) {
  const correction = item.correction ? "" : `<form class="correction-form" data-deviation="${escapeHTML(item.id)}"><label>登记人<input name="actor" required></label><label>整改措施<input name="measure" required></label><label>责任人<input name="assignee" required></label><input type="hidden" name="pointID" value="${escapeHTML(item.pointID)}"><button type="submit">登记整改并生成任务包</button></form>`;
  const options = (state.detail.batch.measurements || []).filter((m) => m.purpose === "retest" && m.pointID === item.pointID && new Date(m.measuredAt) > new Date(item.correction?.recordedAt || 0)).map((m) => `<option value="${escapeHTML(m.id)}">第 ${m.round} 轮 · ${m.peakAmplitudePC} pC · ${formatTime(m.measuredAt)}</option>`).join("");
  const retest = item.correction ? `<form class="retest-form" data-deviation="${escapeHTML(item.id)}"><label>操作人<input name="actor" required></label><label>复验读数（可多选）<select name="measurementIDs" multiple size="4" required>${options}</select></label><label>补充结论<input name="conclusion" placeholder="可留空采用规则结论"></label><button type="submit">按任务包执行复验</button></form>` : "";
  return correction + retest;
}

function bindDeviationForms() {
  document.querySelectorAll(".correction-form").forEach((form) => form.addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(form); mutate(`/deviations/${form.dataset.deviation}/correction`, { actor: v.actor, measure: v.measure, assignee: v.assignee, retestPoints: [v.pointID] }, "correct", "整改措施及复验任务包已保存"); }));
  document.querySelectorAll(".retest-form").forEach((form) => form.addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(form); const ids = [...form.querySelector("[name=measurementIDs]").selectedOptions].map((option) => option.value); mutate(`/deviations/${form.dataset.deviation}/retest`, { actor: v.actor, measurementIDs: ids, conclusion: v.conclusion }, "retest", "定向复验任务已更新"); }));
}

function renderReviewReadiness(readiness) {
  const items = readiness?.checklist || []; $("#reviewReadiness").innerHTML = `<strong>${readiness?.ready ? "复核就绪清单通过" : "复核就绪清单未通过"}</strong>${items.map((item) => `<div>${item.passed ? "✓" : "✕"} ${escapeHTML(item.message)}</div>`).join("")}${readiness?.snapshot ? `<p class="digest">证据快照 ${escapeHTML(readiness.snapshot.digest)} · 版本 ${readiness.snapshot.batchVersion} · 偏差 ${readiness.snapshot.deviationCount}</p>` : ""}`;
  $("#reviewForm [name=evidenceDigest]").value = readiness?.snapshot?.digest || ""; $("#reviewForm [name=evidenceVersion]").value = readiness?.snapshot?.batchVersion || "";
}

function renderReviews(items) { $("#reviewList").innerHTML = items.length ? items.map((review, index) => `<div class="evidence"><strong>复核 ${index + 1} · ${escapeHTML(review.reviewer)}（${escapeHTML(review.role)}）</strong><p>${review.approved ? "同意复归" : "不同意复归"}：${escapeHTML(review.opinion)}</p><p class="muted">证据版本 ${review.evidenceVersion || "—"} · 偏差 ${review.deviationCount ?? "—"} · ${formatTime(review.submittedAt)}</p><p class="mono">${escapeHTML(review.evidenceDigest || "")}</p></div>`).join("") : `<p class="muted">尚无安全复核意见。</p>`; }

function renderCertificate(certificate) {
  if (!certificate) { $("#certificateView").innerHTML = `<p class="muted">证书尚未签发。</p>`; return; }
  const base = `/api/batches/${encodeURIComponent(certificate.batchID)}`;
  $("#certificateView").innerHTML = `<div class="certificate"><h3>复归放行凭据 ${escapeHTML(certificate.certificateVersion)}</h3><p>证书编号：<span class="mono">${escapeHTML(certificate.id)}</span></p><p>复核人：${escapeHTML(certificate.reviewerA)} / ${escapeHTML(certificate.reviewerB)}</p><p>签发时间：${formatTime(certificate.issuedAt)}</p><p class="digest">SHA-256 ${escapeHTML(certificate.evidenceDigest)}</p><div class="button-row"><a href="${base}/certificate" download>下载证书</a><a href="${base}/verification-package" download>下载核验包</a><button type="button" id="verifyCertificate" class="ghost">在线完整性核验</button></div><div id="verificationResult"></div></div>`;
  $("#verifyCertificate").addEventListener("click", () => withAction(async () => { const result = await api(`${base}/verification-package/verify`); $("#verificationResult").innerHTML = `<strong>${result.valid ? "核验有效" : "核验无效"}</strong> · 证书 ${result.certificateValid ? "有效" : "异常"} · 清单 ${result.evidenceListValid ? "有效" : "异常"} · 审计 ${result.auditValid ? "有效" : "异常"}${result.anomalies?.length ? `<ul>${result.anomalies.map((v) => `<li>${escapeHTML(v)}</li>`).join("")}</ul>` : ""}`; if (!result.valid) throw new Error("核验包存在异常项"); }, "证书、证据清单和审计关联均有效"));
}

function renderTimeline(items) { $("#timeline").innerHTML = items.length ? items.map((event) => `<div class="timeline-item"><div class="timeline-index">${event.sequence}</div><div class="timeline-body"><p><strong>${escapeHTML(event.operation)}</strong> · ${escapeHTML(event.actor)}</p><p class="muted">${formatTime(event.occurredAt)} · 批次版本 ${event.version}</p><p class="mono">${escapeHTML(event.hash.slice(0, 20))}…</p></div></div>`).join("") : `<p class="muted">尚无审计事件。</p>`; }

function setAvailability(batch) {
  const sealed = batch.status === "sealed"; $("#freezeForm button[type=submit]").disabled = batch.status !== "draft";
  $("#measurementForm button[type=submit]").disabled = batch.status === "draft" || sealed || batch.reviews.length > 0;
  $("#diagnoseForm button").disabled = sealed || !state.detail.diagnosisReadiness?.ready;
  $("#reviewForm button").disabled = batch.status !== "reviewing" || batch.reviews.length >= 2 || !state.detail.reviewReadiness?.ready;
  $("#issueForm button").disabled = batch.status !== "reviewing" || batch.reviews.length !== 2 || batch.reviews.some((item) => !item.approved);
}

async function mutate(suffix, values, operation, successMessage) { if (!state.detail) return; await withAction(async () => { const batch = state.detail.batch; const payload = { ...values, expectedVersion: batch.version, idempotencyKey: makeKey(operation) }; await api(`/api/batches/${encodeURIComponent(batch.id)}${suffix}`, { method: "POST", body: JSON.stringify(payload) }); await selectBatch(batch.id); await loadBatches(); }, successMessage); }

function pointRow(value = {}) {
  const row = document.createElement("tr"); row.innerHTML = `<td><input name="id" value="${escapeHTML(value.id || `P${$("#pointRows").children.length + 1}`)}" required></td><td><input name="name" value="${escapeHTML(value.name || "电缆试验点")}" required></td><td><input name="location" value="${escapeHTML(value.location || "")}" required></td><td><input name="sensorRangePC" type="number" step="0.1" value="${value.sensorRangePC || 100}" required></td><td><input name="amplitudeLimitPC" type="number" step="0.1" value="${value.amplitudeLimitPC || 20}" required></td><td><input name="trendLimitPercent" type="number" step="0.1" value="${value.trendLimitPercent || 25}" required></td><td><input name="repeatabilityCount" type="number" value="${value.repeatabilityCount || 3}" required></td><td class="row-actions"><button type="button" data-up class="ghost">↑</button><button type="button" data-down class="ghost">↓</button><button type="button" data-remove class="ghost">删</button></td>`;
  row.addEventListener("input", invalidatePreflight); row.querySelector("[data-remove]").addEventListener("click", () => { if ($("#pointRows").children.length > 1) row.remove(); invalidatePreflight(); }); row.querySelector("[data-up]").addEventListener("click", () => { if (row.previousElementSibling) row.parentNode.insertBefore(row, row.previousElementSibling); invalidatePreflight(); }); row.querySelector("[data-down]").addEventListener("click", () => { if (row.nextElementSibling) row.parentNode.insertBefore(row.nextElementSibling, row); invalidatePreflight(); }); return row;
}

function readPoints() { return [...$("#pointRows").children].map((row) => { const v = formObject(row); return { id: v.id, name: v.name, location: v.location, sensorRangePC: Number(v.sensorRangePC), amplitudeLimitPC: Number(v.amplitudeLimitPC), trendLimitPercent: Number(v.trendLimitPercent), repeatabilityCount: Number(v.repeatabilityCount) }; }); }
function invalidatePreflight() { state.scopeDigest = ""; $("#confirmScope").checked = false; $("#scopePreflight").textContent = "内容已变化，请重新预检。"; }

function localDateTime() { const now = new Date(); now.setMinutes(now.getMinutes() - now.getTimezoneOffset()); return now.toISOString().slice(0, 16); }
function measurementRow(value = {}) {
  const row = document.createElement("tr"); row.innerHTML = `<td><input name="id" value="${escapeHTML(value.id || `reading-${crypto.randomUUID()}`)}" required></td><td><select name="pointID" required>${pointOptions(value.pointID)}</select></td><td><input name="round" type="number" min="1" value="${value.round || 1}" required></td><td><input name="measuredAt" type="datetime-local" value="${escapeHTML(value.measuredAt || localDateTime())}" required></td><td><input name="peakAmplitudePC" type="number" min="0" step="0.01" value="${value.peakAmplitudePC ?? ""}" required></td><td><input name="phaseSummary" value="${escapeHTML(value.phaseSummary || "均匀分布，无集中尖峰")}" required></td><td><input name="temperatureC" type="number" step="0.1" value="${value.temperatureC ?? 25}" required></td><td><input name="humidityPercent" type="number" step="0.1" value="${value.humidityPercent ?? 55}" required></td><td><input name="sensorSerial" value="${escapeHTML(value.sensorSerial || "")}" required></td><td><input name="operator" value="${escapeHTML(value.operator || "")}" required></td><td><select name="purpose"><option value="initial">初始</option><option value="retest" ${value.purpose === "retest" ? "selected" : ""}>复验</option></select></td><td class="row-actions"><button type="button" data-copy class="ghost">复制</button><button type="button" data-remove class="ghost">删</button></td>`;
  row.addEventListener("input", summarizeDraftMeasurements); row.addEventListener("change", summarizeDraftMeasurements); row.querySelector("[data-remove]").addEventListener("click", () => { if ($("#measurementRows").children.length > 1) row.remove(); summarizeDraftMeasurements(); }); row.querySelector("[data-copy]").addEventListener("click", () => { const copy = readMeasurementRow(row); copy.id = `reading-${crypto.randomUUID()}`; copy.round++; $("#measurementRows").append(measurementRow(copy)); summarizeDraftMeasurements(); }); return row;
}

function readMeasurementRow(row) { const v = formObject(row); return { id: v.id, pointID: v.pointID, round: Number(v.round), measuredAt: v.measuredAt, peakAmplitudePC: Number(v.peakAmplitudePC), phaseSummary: v.phaseSummary, temperatureC: Number(v.temperatureC), humidityPercent: Number(v.humidityPercent), sensorSerial: v.sensorSerial, operator: v.operator, purpose: v.purpose }; }
function readMeasurements() { return [...$("#measurementRows").children].map(readMeasurementRow).map((item) => ({ ...item, measuredAt: new Date(item.measuredAt).toISOString() })); }
function summarizeDraftMeasurements() { const rows = [...$("#measurementRows").children].map(readMeasurementRow); const grouped = {}; const warnings = []; rows.forEach((item, index) => { const key = `${item.pointID}/${item.purpose}`; grouped[key] ||= []; grouped[key].push(item.round); const point = state.detail?.batch.frozenScope?.points?.find((p) => p.id === item.pointID); if (point && item.peakAmplitudePC > point.amplitudeLimitPC) warnings.push(`第 ${index + 1} 行超过幅值阈值`); }); $("#measurementDraftSummary").textContent = Object.entries(grouped).map(([key, rounds]) => `${key}：${rounds.length} 行，轮次 ${Math.min(...rounds)}-${Math.max(...rounds)}`).join("；") + (warnings.length ? `；异常提示：${warnings.join("、")}` : ""); }

async function lookupMatches() { const form = $("#createForm"); const v = formObject(form); if (!v.cableSection.trim() || !v.circuitName.trim()) return; const result = await api(`/api/batches/matches?cableSection=${encodeURIComponent(v.cableSection)}&circuitName=${encodeURIComponent(v.circuitName)}`); const active = result.active || []; const sealed = result.latestSealed; $("#batchMatches").innerHTML = `${active.length ? active.map((item) => `<div class="match danger">未封存：${escapeHTML(item.id)} · ${escapeHTML(item.statusLabel)} · ${escapeHTML(item.testOwner)} · ${formatTime(item.updatedAt)}</div>`).join("") : "<div>未发现未封存同回路批次。</div>"}${sealed ? `<div class="match">最近封存：${escapeHTML(sealed.id)} · ${escapeHTML(sealed.testOwner)} · ${formatTime(sealed.updatedAt)} <button type="button" class="ghost" data-reuse="${escapeHTML(sealed.id)}">复用资料</button></div>` : ""}`; document.querySelectorAll("[data-reuse]").forEach((button) => button.addEventListener("click", () => { form.elements.sourceBatchID.value = button.dataset.reuse; showToast(`已选择来源批次 ${button.dataset.reuse}`); })); }

$("#createForm").addEventListener("input", (event) => { if (["cableSection", "circuitName"].includes(event.target.name)) { clearTimeout(lookupMatches.timer); lookupMatches.timer = setTimeout(() => lookupMatches().catch((error) => showToast(error.message, true)), 350); } });
$("#createForm").addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(event.currentTarget); withAction(async () => { const payload = { cableSection: v.cableSection, circuitName: v.circuitName, testOwner: v.testOwner, sourceBatchID: v.sourceBatchID, confirmDuplicate: v.confirmDuplicate === "on", idempotencyKey: makeKey("create") }; const batch = await api("/api/batches", { method: "POST", body: JSON.stringify(payload) }); event.currentTarget.reset(); $("#batchMatches").textContent = "输入区段和回路后自动查重。"; await loadBatches(); await selectBatch(batch.id); }, "试验批次已创建"); });

$("#addPoint").addEventListener("click", () => $("#pointRows").append(pointRow()));
$("#preflightScope").addEventListener("click", () => withAction(async () => { const result = await api(`/api/batches/${encodeURIComponent(state.detail.batch.id)}/freeze/preflight`, { method: "POST", body: JSON.stringify({ points: readPoints() }) }); state.scopeDigest = result.scopeDigest; $("#scopePreflight").innerHTML = `<strong>预检通过，规范顺序：${result.points.map((p) => escapeHTML(p.id)).join(" → ")}</strong><p>${escapeHTML(result.rangeSummary)}</p><p class="digest">${escapeHTML(result.scopeDigest)}</p>`; }, "冻结预检通过，请二次确认摘要"));
$("#freezeForm").addEventListener("submit", (event) => { event.preventDefault(); if (!state.scopeDigest || !$("#confirmScope").checked) { showToast("请先执行冻结预检并确认摘要", true); return; } const v = formObject(event.currentTarget); mutate("/freeze", { actor: v.actor, points: readPoints(), preflightScopeDigest: state.scopeDigest, confirmed: true }, "freeze", "试验边界已原子冻结"); });

$("#addMeasurement").addEventListener("click", () => { $("#measurementRows").append(measurementRow()); summarizeDraftMeasurements(); });
$("#measurementForm").addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(event.currentTarget); mutate("/measurements/batch", { actor: v.actor, measurements: readMeasurements() }, "measurements", "全部现场读数已原子保存"); });
$("#diagnoseForm").addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(event.currentTarget); mutate("/diagnose", { actor: v.actor }, "diagnose", "规则诊断报告已生成"); });
$("#reviewForm").addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(event.currentTarget); mutate("/reviews", { actor: v.actor, reviewer: v.reviewer, role: v.role, approved: v.approved === "true", opinion: v.opinion, evidenceDigest: v.evidenceDigest, evidenceVersion: Number(v.evidenceVersion) }, "review", "复核意见及证据快照已锁定"); });
$("#issueForm").addEventListener("submit", (event) => { event.preventDefault(); const v = formObject(event.currentTarget); mutate("/issue", { actor: v.actor }, "issue", "复归证书及核验包已封存"); });
$("#refreshBatches").addEventListener("click", () => withAction(loadBatches, "批次列表已刷新"));

function outcomeLabel(value) { return ({ passed: "通过", triggered: "触发", insufficient: "证据不足" })[value] || value; }
function escapeHTML(value) { return String(value ?? "").replace(/[&<>'"]/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[char]); }
function formatTime(value) { return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "—"; }

$("#pointRows").append(pointRow({ id: "P1", name: "终端接头", location: "A 相终端" }));
$("#measurementRows").append(measurementRow()); summarizeDraftMeasurements();
Promise.all([loadHealth(), loadBatches()]).catch((error) => showToast(error.message, true));
