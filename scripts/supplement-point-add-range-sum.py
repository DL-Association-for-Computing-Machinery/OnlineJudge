#!/usr/bin/env python3
"""Make the fixed point-add-range-sum revision with site-authored boundary cases."""

import sys

sys.dont_write_bytecode = True

from library_checker_supplement import main


if __name__ == "__main__":
    main("point_add_range_sum")
