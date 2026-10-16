"""IDA 9.4 唯讀來源的窄匯出；重跑入口見 docs/atari-st/walker-timing-spec.md。"""
import hashlib
import json
from pathlib import Path

import ida_auto
import ida_bytes
import ida_funcs
import ida_kernwin
import ida_nalt
import ida_name
import ida_pro
import ida_ua
import idautils
import idc

ida_auto.auto_wait()
source = Path(ida_nalt.get_input_file_path())
digest = hashlib.sha256(source.read_bytes()).hexdigest()
if digest != "f304c5a5384a5fedec9724ee33bb1991160942923dd154907fe67c86b10cbb7c":
    raise RuntimeError("GEMBIND 來源身分不同")
# 經原始讀寫端、Talos 步長與同輸入位置序列審查；不從未分級散文自動匯入。
reviewed = {
    0xb80: "讀取記錄 word +3 的目標 X",
    0xb84: "讀取記錄 word +4 的目標 Y",
    0xb88: "讀取記錄 word +1 的目前 X 錨點",
    0xb8c: "讀取記錄 word +2 的目前 Y 錨點",
    0xbc0: "計算目標 X 減目前 X；後續限制步長至 ±2",
    0xbd8: "計算目標 Y 減目前 Y；後續限制步長至 ±2",
    0xcc8: "固定圖樣 handler 成功後寫回目前 X 錨點",
    0xccc: "固定圖樣 handler 成功後寫回目前 Y 錨點",
}
rows = []
for first, last in [(0xb40, 0xc90), (0xcaa, 0xd70)]:
    at = first
    while at < last:
        length = ida_ua.create_insn(at)
        if not length:
            raise RuntimeError(f"IDA 無法解碼 {at:#x}")
        func = ida_funcs.get_func(at)
        rows.append({
            "ida_address": hex(at), "disk_raw_offset": hex(0x2fe00 + at),
            "bytes": ida_bytes.get_bytes(at, length).hex(),
            "original_name": ida_name.get_name(at),
            "original_function": ida_funcs.get_func_name(at) if func else None,
            "instruction": idc.generate_disasm_line(at, 0),
            "xrefs_from": [{"address": hex(x.to), "type": x.type} for x in idautils.XrefsFrom(at)],
            "semantic": reviewed.get(at), "grade": "proven" if at in reviewed else "unknown",
            "warning": None if at in reviewed else "只核定指令解碼；未附加已證實語意",
            "evidence": ("docs/atari-st/walker-timing-spec.md：錨點讀寫端、Talos 81 案步長及 810 次位置比較"
                         if at in reviewed else "本檔輸入雜湊／IDA 9.4／原始 bytes"),
        })
        at += length
report = {"schema": 1, "input_file": source.name, "input_sha256": digest,
          "tool": "IDA Pro", "version": ida_kernwin.get_kernel_version(),
          "address_space": "GEMBIND.bin binary base 0; disk raw = 0x2fe00 + IDA address",
          "rows": rows}
Path("/out/native-walker.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
ida_pro.qexit(0)
