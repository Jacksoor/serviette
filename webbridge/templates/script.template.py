#!/usr/bin/python3

"""
This is an example script using Python.
"""

import sys
sys.path.insert(0, '/usr/lib/k4')

# Import the k4 library.
import k4


# Create a new client.
client = k4.Client()

# Input.
inp = sys.stdin.read()

# Open some persistent storage.
try:
    with open('/mnt/storage/number_of_greetings', 'r') as f:
        num_his = int(f.read())
except IOError:
    num_his = 0

# Greet the user!
print('Hi, {}! I\'ve said "hi" {} times! You said "{}"!'.format(
    client.context.mention, num_his, inp))

# Increment the number of his and put it back into persistent storage.
with open('/mnt/storage/number_of_greetings', 'w') as f:
    f.write(str(num_his + 1))
