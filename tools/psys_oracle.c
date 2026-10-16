/*
 * p-code 對拍 oracle：用 laanwj/sundog 的 C 直譯器執行一小段 p-code，把結果印成 JSON。
 *
 * 用途是給 Atari Talos 的驗收當第二個獨立實作。那邊執行的是原版磁碟上的 68000 直譯器
 * （SYSTEM.INTERP）；這邊執行的是同一套 p-machine 的 C 重寫。同一段 p-code 兩邊跑出
 * 相同結果，才算對得起來——單獨一邊只能證明自己自洽。
 *
 * 直譯器本體與 helper 全部來自 laanwj/sundog（MIT），本檔只建立最小執行狀態並把結果
 * 印出來，不修改直譯器的任何行為。單步靠 trace hook：它在每條指令之前被呼叫，
 * 額度用完就 psys_stop。
 *
 * 這支不載入磁碟、不需要遊戲檔案——只有一塊記憶體、一段 p-code 與初始堆疊。
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "psys_state.h"
#include "psys_interpreter.h"
#include "psys_helpers.h"

#define MEM_SIZE 0x10000
#define STACK_TOP 0xff00
#define PCODE_AT 0x1000
#define FRAME_AT 0x8000

static int budget;

static void step_limit(struct psys_state *s, void *data)
{
    (void)data;
    if (budget-- <= 0) {
        psys_stop(s);
    }
}

static int parse_hex_bytes(const char *text, psys_byte *out, int max)
{
    int n = 0;
    while (*text && n < max) {
        while (*text == ' ' || *text == ',') text++;
        if (!*text) break;
        unsigned value;
        if (sscanf(text, "%2x", &value) != 1) return -1;
        out[n++] = (psys_byte)value;
        text += 2;
    }
    return n;
}

static void usage(void)
{
    fprintf(stderr,
        "用法：psys_oracle <pcode-hex> [--ops N] [--push v]... [--local n=v]...\n"
        "  <pcode-hex>  p-code 位元組，例如 \"22 07 d7 14\"\n"
        "  --ops N      執行幾條 p-code 指令（預設：全部，上限 1000）\n"
        "  --push v     預先推上運算元堆疊的 word，可重複，先給的在較高位址\n"
        "  --local n=v  把活動記錄第 n 格（1-based）設成 v\n"
        "  --poke a=v   把位址 a 的 byte 設成 v，可重複\n");
}

int main(int argc, char **argv)
{
    if (argc < 2) {
        usage();
        return 2;
    }

    psys_byte pcode[256];
    int pcode_len = parse_hex_bytes(argv[1], pcode, sizeof(pcode));
    if (pcode_len <= 0) {
        fprintf(stderr, "p-code 解析失敗\n");
        return 2;
    }

    struct psys_state state;
    memset(&state, 0, sizeof(state));
    state.memory = calloc(1, MEM_SIZE);
    state.mem_size = MEM_SIZE;
    state.sp = STACK_TOP;
    state.mp = FRAME_AT;
    state.base = FRAME_AT;
    state.curseg = 0;
    state.ipc = PCODE_AT;
    state.trace = step_limit;
    memcpy(state.memory + PCODE_AT, pcode, pcode_len);

    budget = 1000;
    int pushed = 0;
    for (int i = 2; i < argc; i++) {
        if (!strcmp(argv[i], "--ops") && i + 1 < argc) {
            budget = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "--push") && i + 1 < argc) {
            psys_push(&state, (psys_word)strtoul(argv[++i], NULL, 0));
            pushed++;
        } else if (!strcmp(argv[i], "--local") && i + 1 < argc) {
            int slot; unsigned value;
            if (sscanf(argv[++i], "%d=%i", &slot, &value) != 2) {
                fprintf(stderr, "--local 要寫成 n=value\n");
                return 2;
            }
            /* 活動記錄的區域變數：mp + 8 + n*2，與 SYSTEM.INTERP 的 8(a0,d0.l) 一致。 */
            psys_stw(&state, W(state.mp + 8, slot), (psys_word)value);
        } else if (!strcmp(argv[i], "--poke") && i + 1 < argc) {
            unsigned at, value;
            if (sscanf(argv[++i], "%i=%i", &at, &value) != 2) {
                fprintf(stderr, "--poke 要寫成 addr=value\n");
                return 2;
            }
            if (at >= MEM_SIZE) {
                fprintf(stderr, "--poke 位址超出記憶體\n");
                return 2;
            }
            state.memory[at] = (psys_byte)value;
        } else {
            usage();
            return 2;
        }
    }

    psys_word sp_before = state.sp;
    psys_interpreter(&state);

    printf("{\n");
    printf("  \"pcodeBytes\": %d,\n", pcode_len);
    printf("  \"ipcAdvanced\": %d,\n", (int)(state.ipc - PCODE_AT));
    printf("  \"spBefore\": \"0x%04x\",\n", sp_before);
    printf("  \"spAfter\": \"0x%04x\",\n", state.sp);
    printf("  \"stack\": [");
    for (psys_word at = state.sp; at < STACK_TOP; at += 2) {
        printf("%s%d", at == state.sp ? "" : ", ", psys_ldw(&state, at));
    }
    printf("],\n");
    printf("  \"locals\": {");
    int first = 1;
    for (int slot = 1; slot <= 16; slot++) {
        psys_word value = psys_ldw(&state, W(state.mp + 8, slot));
        if (!value) continue;
        printf("%s\"%d\": %d", first ? "" : ", ", slot, value);
        first = 0;
    }
    printf("}\n");
    printf("}\n");

    free(state.memory);
    return 0;
}
