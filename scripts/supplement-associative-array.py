#!/usr/bin/env python3
"""Make the fixed associative-array revision with site-authored boundary cases."""

import sys

sys.dont_write_bytecode = True

from library_checker_supplement import main


if __name__ == "__main__":
    main("associative_array")
