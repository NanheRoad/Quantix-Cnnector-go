from __future__ import annotations

import argparse
import threading
import time
from dataclasses import dataclass

from pymodbus.datastore import ModbusDeviceContext, ModbusSequentialDataBlock, ModbusServerContext
from pymodbus.server import StartTcpServer


@dataclass
class RuntimeConfig:
    host: str
    port: int
    slave_id: int
    address: int
    scale: int
    start_weight: float
    auto_step: float
    auto_interval: float
    auto_start: bool
    tare_control_addr: int
    zero_control_addr: int


class WeightSimulator:
    def __init__(self, device_ctx: ModbusDeviceContext, cfg: RuntimeConfig):
        self._ctx = device_ctx
        self._cfg = cfg
        self._lock = threading.Lock()
        self._gross_weight = float(cfg.start_weight)
        self._tare_offset = 0.0
        self._auto_enabled = bool(cfg.auto_start)
        self._auto_step = float(cfg.auto_step)
        self._auto_interval = float(cfg.auto_interval)
        self._running = True
        self._write_output_weight_unlocked(self._net_weight_unlocked())

    def stop(self) -> None:
        with self._lock:
            self._running = False

    def is_running(self) -> bool:
        with self._lock:
            return self._running

    def set_weight(self, value: float) -> None:
        with self._lock:
            self._gross_weight = float(value)
            self._write_output_weight_unlocked(self._net_weight_unlocked())

    def add_weight(self, delta: float) -> None:
        with self._lock:
            self._gross_weight += float(delta)
            self._write_output_weight_unlocked(self._net_weight_unlocked())

    def tare(self) -> None:
        with self._lock:
            self._tare_offset = self._gross_weight
            self._write_output_weight_unlocked(self._net_weight_unlocked())

    def zero(self) -> None:
        with self._lock:
            self._gross_weight = 0.0
            self._tare_offset = 0.0
            self._write_output_weight_unlocked(self._net_weight_unlocked())

    def set_auto_enabled(self, enabled: bool) -> None:
        with self._lock:
            self._auto_enabled = bool(enabled)

    def set_auto_step(self, step: float) -> None:
        with self._lock:
            self._auto_step = float(step)

    def set_auto_interval(self, interval_sec: float) -> None:
        with self._lock:
            self._auto_interval = max(float(interval_sec), 0.05)

    def snapshot(self) -> dict[str, float | bool]:
        with self._lock:
            return {
                "gross_weight": self._gross_weight,
                "tare_offset": self._tare_offset,
                "net_weight": self._net_weight_unlocked(),
                "auto_enabled": self._auto_enabled,
                "auto_step": self._auto_step,
                "auto_interval": self._auto_interval,
            }

    def auto_loop(self) -> None:
        while True:
            with self._lock:
                if not self._running:
                    return
                enabled = self._auto_enabled
                step = self._auto_step
                interval = self._auto_interval
            if enabled:
                self.add_weight(step)
            time.sleep(interval)

    def control_loop(self) -> None:
        while True:
            with self._lock:
                if not self._running:
                    return
                tare_addr = self._cfg.tare_control_addr
                zero_addr = self._cfg.zero_control_addr

                tare_cmd = self._read_control_unlocked(tare_addr)
                zero_cmd = self._read_control_unlocked(zero_addr)
                if tare_cmd:
                    self._tare_offset = self._gross_weight
                    self._clear_control_unlocked(tare_addr)
                    self._write_output_weight_unlocked(self._net_weight_unlocked())
                if zero_cmd:
                    self._gross_weight = 0.0
                    self._tare_offset = 0.0
                    self._clear_control_unlocked(zero_addr)
                    self._write_output_weight_unlocked(self._net_weight_unlocked())
            time.sleep(0.05)

    def _net_weight_unlocked(self) -> float:
        return self._gross_weight - self._tare_offset

    def _write_output_weight_unlocked(self, weight: float) -> None:
        # raw = kg * scale, encoded as signed int32 then split to two uint16 registers.
        raw = int(round(weight * self._cfg.scale))
        if raw < 0:
            raw &= 0xFFFFFFFF
        hi = (raw >> 16) & 0xFFFF
        lo = raw & 0xFFFF

        addr = self._cfg.address
        # Input registers (FC4)
        self._ctx.setValues(4, addr, [hi, lo])
        # Holding registers (FC3) mirror
        self._ctx.setValues(3, addr, [hi, lo])

    def _read_control_unlocked(self, addr: int) -> bool:
        hr = self._ctx.getValues(3, addr, count=1)
        co = self._ctx.getValues(1, addr, count=1)
        hr_value = int(hr[0]) if hr else 0
        co_value = int(bool(co[0])) if co else 0
        return bool(hr_value != 0 or co_value != 0)

    def _clear_control_unlocked(self, addr: int) -> None:
        self._ctx.setValues(3, addr, [0])
        self._ctx.setValues(1, addr, [0])


def build_server_context(cfg: RuntimeConfig) -> tuple[ModbusServerContext, ModbusDeviceContext]:
    block_di = ModbusSequentialDataBlock(0, [0] * 256)
    block_co = ModbusSequentialDataBlock(0, [0] * 256)
    block_hr = ModbusSequentialDataBlock(0, [0] * 256)
    block_ir = ModbusSequentialDataBlock(0, [0] * 256)
    device_ctx = ModbusDeviceContext(di=block_di, co=block_co, hr=block_hr, ir=block_ir)
    server_ctx = ModbusServerContext(devices={cfg.slave_id: device_ctx}, single=False)
    return server_ctx, device_ctx


def parse_args() -> RuntimeConfig:
    parser = argparse.ArgumentParser(description="Modbus TCP weight simulator (standalone)")
    parser.add_argument("--host", default="127.0.0.1", help="Bind host (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=1502, help="Bind port (default: 1502)")
    parser.add_argument("--slave-id", type=int, default=1, help="Slave id / unit id (default: 1)")
    parser.add_argument("--address", type=int, default=0, help="Register start address (default: 0)")
    parser.add_argument("--scale", type=int, default=1000, help="Raw scale: raw=weight*scale (default: 1000)")
    parser.add_argument("--start-weight", type=float, default=0.0, help="Initial weight in kg (default: 0)")
    parser.add_argument("--auto-step", type=float, default=0.1, help="Auto increment step in kg (default: 0.1)")
    parser.add_argument("--auto-interval", type=float, default=1.0, help="Auto increment interval sec (default: 1.0)")
    parser.add_argument("--auto-start", action="store_true", help="Start with auto increment enabled")
    parser.add_argument("--tare-control-addr", type=int, default=100, help="Tare control register/coil address")
    parser.add_argument("--zero-control-addr", type=int, default=101, help="Zero control register/coil address")
    ns = parser.parse_args()
    return RuntimeConfig(
        host=ns.host,
        port=ns.port,
        slave_id=ns.slave_id,
        address=ns.address,
        scale=max(int(ns.scale), 1),
        start_weight=float(ns.start_weight),
        auto_step=float(ns.auto_step),
        auto_interval=max(float(ns.auto_interval), 0.05),
        auto_start=bool(ns.auto_start),
        tare_control_addr=max(int(ns.tare_control_addr), 0),
        zero_control_addr=max(int(ns.zero_control_addr), 0),
    )


def print_boot_info(cfg: RuntimeConfig) -> None:
    print("=" * 72)
    print("Modbus TCP Test Server (standalone)")
    print("=" * 72)
    print(f"listen       : {cfg.host}:{cfg.port}")
    print(f"slave_id     : {cfg.slave_id}")
    print(f"reg address  : {cfg.address} (2 regs: hi/lo)")
    print(f"tare control : address {cfg.tare_control_addr} (write coil/register non-zero to trigger)")
    print(f"zero control : address {cfg.zero_control_addr} (write coil/register non-zero to trigger)")
    print(f"raw formula  : raw = round(weight_kg * {cfg.scale})")
    print("mapping      : FC4 input registers + FC3 holding registers")
    print("-" * 72)
    print("Quantix template hints:")
    print("  action      : modbus.read_input_registers")
    print(f"  slave_id    : {cfg.slave_id}")
    print(f"  address     : {cfg.address}")
    print("  count       : 2")
    print("  parse expr  : (registers[0] * 65536 + registers[1]) / 1000")
    print("-" * 72)
    print("commands:")
    print("  set <kg>            # set weight")
    print("  add <kg>            # add delta")
    print("  auto on|off         # toggle auto increment")
    print("  step <kg>           # set auto step")
    print("  interval <sec>      # set auto interval")
    print("  show                # print current state")
    print("  tare                # local tare trigger")
    print("  zero                # local zero trigger")
    print("  help                # show commands")
    print("  quit                # exit")
    print("=" * 72)


def run_console(sim: WeightSimulator) -> None:
    while sim.is_running():
        try:
            line = input("> ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\nexit requested")
            sim.stop()
            return

        if not line:
            continue
        parts = line.split()
        cmd = parts[0].lower()

        try:
            if cmd in {"quit", "exit", "q"}:
                sim.stop()
                return
            if cmd == "help":
                print("set <kg> | add <kg> | auto on/off | step <kg> | interval <sec> | show | quit")
                continue
            if cmd == "set" and len(parts) == 2:
                sim.set_weight(float(parts[1]))
                print_state(sim)
                continue
            if cmd == "add" and len(parts) == 2:
                sim.add_weight(float(parts[1]))
                print_state(sim)
                continue
            if cmd == "auto" and len(parts) == 2:
                flag = parts[1].lower() in {"on", "1", "true", "yes"}
                sim.set_auto_enabled(flag)
                print_state(sim)
                continue
            if cmd == "step" and len(parts) == 2:
                sim.set_auto_step(float(parts[1]))
                print_state(sim)
                continue
            if cmd == "interval" and len(parts) == 2:
                sim.set_auto_interval(float(parts[1]))
                print_state(sim)
                continue
            if cmd == "show":
                print_state(sim)
                continue
            if cmd == "tare":
                sim.tare()
                print_state(sim)
                continue
            if cmd == "zero":
                sim.zero()
                print_state(sim)
                continue

            print("invalid command, type 'help'")
        except ValueError:
            print("invalid number format")


def print_state(sim: WeightSimulator) -> None:
    s = sim.snapshot()
    print(
        "gross={:.3f}kg tare_offset={:.3f}kg net={:.3f}kg auto={} step={:.3f}kg interval={:.2f}s".format(
            float(s["gross_weight"]),
            float(s["tare_offset"]),
            float(s["net_weight"]),
            bool(s["auto_enabled"]),
            float(s["auto_step"]),
            float(s["auto_interval"]),
        )
    )


def run_server(server_ctx: ModbusServerContext, cfg: RuntimeConfig) -> None:
    StartTcpServer(context=server_ctx, address=(cfg.host, cfg.port))


def main() -> None:
    cfg = parse_args()
    server_ctx, device_ctx = build_server_context(cfg)
    sim = WeightSimulator(device_ctx=device_ctx, cfg=cfg)

    print_boot_info(cfg)
    print_state(sim)

    auto_thread = threading.Thread(target=sim.auto_loop, name="modbus-auto-loop", daemon=True)
    auto_thread.start()

    control_thread = threading.Thread(target=sim.control_loop, name="modbus-control-loop", daemon=True)
    control_thread.start()

    server_thread = threading.Thread(
        target=run_server,
        args=(server_ctx, cfg),
        name="modbus-tcp-server",
        daemon=True,
    )
    server_thread.start()

    run_console(sim)
    print("bye")


if __name__ == "__main__":
    main()
