"""IDA 9.4：匯出SRS及故障定位；入口見Banville規格冷凍人來源段。"""
import hashlib
import json
from pathlib import Path
import ida_auto
import ida_bytes
import ida_kernwin
import ida_nalt
import ida_pro
import ida_ua
import ida_name
import ida_funcs
import idautils
import idc

ida_auto.auto_wait()
raw = Path(ida_nalt.get_input_file_path()).read_bytes()
assert hashlib.sha256(raw).hexdigest() == "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38"
rows = []
for label, start, end in (("SRS", int.from_bytes(raw[0xec+0xbc*2:0xee+0xbc*2], "big"), None), ("fault-handler", 0x31a, 0x380), ("stack-check", 0x380, 0x3ce)):
    at = start
    for _ in range(100):
        n = ida_ua.create_insn(at)
        assert n
        line = idc.generate_disasm_line(at, 0)
        rows.append(dict(label=label, address=hex(at), bytes=ida_bytes.get_bytes(at, n).hex(), instruction=line,
                         original_name=ida_name.get_name(at), original_function=ida_funcs.get_func_name(at),
                         evidenceLevel="unknown", xrefs_from=[dict(address=hex(x.to), type=x.type) for x in idautils.XrefsFrom(at)]))
        at += n
        if (end is not None and at >= end) or (end is None and "jmp" in line):
            break
Path("/out/cryo-interp.json").write_text(json.dumps(dict(input_sha256=hashlib.sha256(raw).hexdigest(),
    version=ida_kernwin.get_kernel_version(), addressSpace="SYSTEM.INTERP binary base0", rows=rows), ensure_ascii=False, indent=2)+"\n")
ida_pro.qexit(0)
