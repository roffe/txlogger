#ifndef _LZHUF_H
#define _LZHUF_H

#ifdef __cplusplus
extern "C"
{
#endif

#include <stdio.h>
#include <stdint.h>

#if defined(LZHUF)

    unsigned int Decode(unsigned char *in, unsigned char *out);

#ifdef __cplusplus
}
#endif

#endif /* defined(LZHUF) */
#endif /* _LZHUF_H */
