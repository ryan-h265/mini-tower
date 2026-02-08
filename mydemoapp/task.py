import os
import time

friend = os.getenv("friend")
foe = os.getenv("foe")

for i in range(int(os.getenv("count", "10"))):
    print(f"[{i}] Hello, {friend}! Boo to {foe}")
    time.sleep(1)

time.sleep(1800)